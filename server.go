package gserver

import (
	"context"
	"database/sql"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
	rpc "github.com/rveen/ogdl/ogdlrf"
	"github.com/rveen/session"

	"github.com/rveen/certmagic"
	// A certmagic version with Shutdown() function!
)

type login interface {
	Auth(r *http.Request, s *Server) (string, error)
	AuthByCookie(r *http.Request) string
	SetCookie(w http.ResponseWriter, user string)
}

type contextService interface {
	GlobalContext(*Server)
}

type domainConfig interface {
	GetConfig(*ogdl.Graph, string, int) *ogdl.Graph
}

type Server struct {
	Host           string
	SecureHost     string
	Hosts          []string
	Config         *ogdl.Graph
	HostContexts   map[string]*ogdl.Graph
	Context        *ogdl.Graph
	Root           *fn.FNode
	DocRoot        string
	UploadDir      string
	Sessions       session.Manager
	DefaultUser    string
	UserDb         *sql.DB
	MaxSessions    int
	Plugins        []string
	Login          login
	ContextService contextService
	DomainConfig   domainConfig
	Templates      map[string]*ogdl.Graph
	Multi          bool
	server         *http.Server
}

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
func New(host string) (*Server, error) {

	srv := Server{}

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = host

	// Server configuration file (optional)
	srv.Config = ogdl.FromFile(".conf/config.ogdl")
	if srv.Config == nil {
		return nil, errors.New("missing .conf/config.ogdl file")
	}

	// Base context for templates (optional)
	srv.Context = ogdl.FromFile(".conf/context.ogdl")
	if srv.Context == nil {
		return nil, errors.New("missing .conf/context.ogdl file")
	}

	// Preload templates
	tpls := srv.Config.Get("templates")
	srv.Templates = make(map[string]*ogdl.Graph)
	if tpls.Len() > 0 {
		for _, tpl := range tpls.Out {
			srv.Templates[tpl.ThisString()] = ogdl.NewTemplate(tpl.String())
		}
	}

	// Register remote functions
	rfs := srv.Config.Get("ogdlrf")
	if rfs != nil {
		for _, rf := range rfs.Out {
			name := rf.ThisString()
			host := rf.Get("host").String()
			proto := rf.Get("protocol").Int64(2)
			log.Println("remote function registered:", name, host, proto)
			f := rpc.Client{Host: host, Timeout: 1, Protocol: int(proto)}
			srv.Context.Set(name, f.Call)
		}
	}

	srv.Hosts = append(srv.Hosts, srv.Host)

	// Default Auth
	// srv.Login = LoginService{}

	// Default context builder
	srv.ContextService = nil

	// Session manager
	// session.Global.Close()
	srv.Sessions = session.NewCookieManagerOptions(session.NewInMemStore(), &session.CookieMngrOptions{AllowHTTP: true, CookieMaxAge: time.Hour * 24 * 90})
	srv.MaxSessions = 10000

	return &srv, nil
}

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
func NewMulti() (*Server, error) {

	srv := Server{}
	srv.Multi = true

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = ":80"

	srv.MaxSessions = 10000

	// Server configuration file (optional)
	srv.Config = ogdl.FromFile(".conf/config.ogdl")
	if srv.Config == nil {
		srv.Config = ogdl.New(nil)
	}

	// Base context for templates
	// Each host gets its own
	files, _ := ioutil.ReadDir(".")
	srv.HostContexts = make(map[string]*ogdl.Graph)

	for _, f := range files {

		// make sure 'file' and other similar entries are filtered out
		if f.Name()[0] == '.' || f.Name()[0] == '_' || !strings.Contains(f.Name(), ".") {
			continue
		}
		fi, err := os.Stat(f.Name())
		if err != nil {
			continue
		}

		if !fi.IsDir() {
			continue
		}

		srv.HostContexts[f.Name()] = ogdl.FromFile(f.Name() + "/.conf/context.ogdl")
		log.Println("context loaded for host", f.Name())

		srv.Hosts = append(srv.Hosts, f.Name())
	}

	// Preload templates
	tpls := srv.Config.Get("templates")
	srv.Templates = make(map[string]*ogdl.Graph)
	if tpls.Len() > 0 {
		for _, tpl := range tpls.Out {
			srv.Templates[tpl.ThisString()] = ogdl.NewTemplate(tpl.String())
		}
	}

	// Register remote functions
	rfs := srv.Config.Get("ogdlrf")
	if rfs != nil {
		for _, rf := range rfs.Out {
			name := rf.ThisString()
			host := rf.Get("host").String()
			proto := rf.Get("protocol").Int64(2)
			log.Println("remote function registered:", name, host, proto)
			f := rpc.Client{Host: host, Timeout: 1, Protocol: int(proto)}
			srv.Context.Set(name, f.Call)
		}
	}

	// Default Auth
	// srv.Login = LoginService{}

	// Default context builder
	srv.ContextService = nil

	// Session manager
	session.Global.Close()
	srv.Sessions = session.NewCookieManagerOptions(session.NewInMemStore(), &session.CookieMngrOptions{AllowHTTP: true, CookieMaxAge: time.Hour * 24 * 90})

	return &srv, nil
}

func (srv *Server) Serve(secure bool, timeout int, router fr.Router, email string) {
	// Serve either HTTP or HTTPS.
	// In case of HTTPS, all requests to HTTP are redirected.
	//
	// HTTPS served with the aid of Let's Encrypt.

	if secure {

		// read and agree to your CA's legal documents
		certmagic.DefaultACME.Agreed = true

		// provide an email address
		certmagic.DefaultACME.Email = email

		// use the staging endpoint while we're developing
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
		err := certmagic.HTTPS(srv.Hosts, router)
		if err != nil {
			log.Println(err.Error())
		}

	} else {

		if srv.Host == "" {
			srv.Host = ":80"
		}
		server := &http.Server{Addr: srv.Host, Handler: router}
		log.Println("starting non-SSL, host:", srv.Host)
		srv.server = server
		server.ListenAndServe()
	}
}

func (srv *Server) Shutdown() {
	log.Println("Shutting down server")
	if srv.server != nil {
		srv.server.Shutdown(context.Background())
	}
	certmagic.Shutdown()
}

func serveTLS(host string, srv *http.Server) {
	if host == "localhost" || host == "" {
		log.Println(srv.ListenAndServeTLS(".certs/cert.pem", ".certs/key.pem"))
	} else {
		log.Println(srv.ListenAndServeTLS("", ""))
	}
}
