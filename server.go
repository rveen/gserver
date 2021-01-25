package gserver

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rveen/golib/fs"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/icza/session"
	"github.com/rveen/ogdl"
	rpc "github.com/rveen/ogdl/ogdlrf"
	"golang.org/x/crypto/acme/autocert"
)

type login interface {
	Auth(r *http.Request, s *Server) (string, error)
	AuthByCookie(r *http.Request) string
	SetCookie(w http.ResponseWriter, user string)
}

type contextService interface {
	SessionContext(*ogdl.Graph, *Server)
	GlobalContext(*Server)
}

type domainConfig interface {
	GetConfig(*ogdl.Graph, string, int) *ogdl.Graph
}

type Server struct {
	Host           string
	SecureHost     string
	Config         *ogdl.Graph
	HostContexts   map[string]*ogdl.Graph
	Context        *ogdl.Graph
	Root           fs.FileSystem
	DocRoot        string
	UploadDir      string
	Sessions       session.Manager
	Plugins        []string
	Login          login
	ContextService contextService
	DomainConfig   domainConfig
}

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
//
func New() (*Server, error) {

	srv := Server{}

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = ":8080"

	// Server configuration file (optional)
	srv.Config = ogdl.FromFile(".conf/config.ogdl")
	if srv.Config == nil {
		srv.Config = ogdl.New(nil)
	}

	// Base context for templates (optional)
	srv.Context = ogdl.FromFile(".conf/context.ogdl")

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

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
//
func NewMulti() (*Server, error) {

	srv := Server{}

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = ":8080"

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
		if f.Name() == ".conf" {
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

func (srv *Server) Serve(host_port string, secure bool, timeout int, router fr.Router) {
	// Serve either HTTP or HTTPS.
	// In case of HTTPS, all requests to HTTP are redirected.
	//
	// HTTPS served with the aid of Let's Encrypt.

	if secure {

		hostname := host_port
		h := strings.Split(host_port, ":")
		if len(h) == 2 {
			hostname = h[0]
		}

		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(hostname),
			Cache:      autocert.DirCache(".certs"), //folder for storing certificates
		}

		shttp := &http.Server{
			Addr:         host_port,
			Handler:      router,
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}
		go serveTLS(hostname, shttp)

		s := &http.Server{
			Addr:         hostname + ":80",
			Handler:      certManager.HTTPHandler(nil), //http.HandlerFunc(redirect),
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
		}
		log.Println("starting SSL (with redirect from non-SSL). Hostname is", hostname)
		s.ListenAndServe()
	} else {
		log.Println(http.ListenAndServe(host_port, router))
	}
}

func redirect(w http.ResponseWriter, r *http.Request) {

	target := "https://" + r.Host + r.URL.Path
	if len(r.URL.RawQuery) != 0 {
		target += "?" + r.URL.RawQuery
	}
	log.Printf("redirect to: %s", target)
	http.Redirect(w, r, target,
		// see @andreiavrammsd comment: often 307 > 301
		http.StatusPermanentRedirect)
}

func serveTLS(host string, srv *http.Server) {
	if host == "localhost" || host == "" {
		log.Println(srv.ListenAndServeTLS(".certs/cert.pem", ".certs/key.pem"))
	} else {
		log.Println(srv.ListenAndServeTLS("", ""))
	}
}
