package gserver

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"plugin"
	"reflect"
	"strings"
	"time"
	"unsafe"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/icza/session"
	"github.com/rveen/gserver/files"
	"github.com/rveen/ogdl"
	rpc "github.com/rveen/ogdl/ogdlrf"
	"golang.org/x/crypto/acme/autocert"
)

type login interface {
	Auth(r *http.Request, s *Server) (string, error)
}

type contextService interface {
	Load(*ogdl.Graph, *Server)
}

type Server struct {
	Host           string
	SecureHost     string
	Config         *ogdl.Graph
	Context        *ogdl.Graph
	Root           *files.Files
	DocRoot        string
	UploadDir      string
	Sessions       session.Manager
	Plugins        []string
	Login          login
	ContextService contextService
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
		srv.Config = ogdl.New()
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
	srv.Login = LoginService{}

	// Default context builder
	srv.ContextService = ContextService{}

	// Load plugins

	pluginDir := ".conf/plugin/"

	pp, err := ioutil.ReadDir(pluginDir)
	if err == nil {

		for _, p := range pp {
			if !strings.HasSuffix(p.Name(), ".so") {
				continue
			}
			key := p.Name()[0 : len(p.Name())-3]
			symbol := strings.Title(key)

			pi, err := plugin.Open(pluginDir + p.Name())
			if err == nil {
				log.Println("plugin loaded:", p.Name())
				// inspectPlugin(pi)

				log.Println(" - Expected symbol", symbol)

				fn, err := pi.Lookup(symbol)
				if err == nil {
					log.Println(" - Symbol found! Bind to", key)
					srv.Context.Set(key, fn)
					log.Println(" - Symbol kind", reflect.ValueOf(fn).Kind().String())
					srv.Plugins = append(srv.Plugins, key)
				}
			} else {
				log.Println(err)
			}
		}
	}

	// Session manager
	session.Global.Close()
	srv.Sessions = session.NewCookieManagerOptions(session.NewInMemStore(), &session.CookieMngrOptions{AllowHTTP: true})

	// File cache
	srv.Root = files.New(srv.DocRoot)

	return &srv, nil
}

func (srv *Server) Serve(host string, secure bool, timeout int, router fr.Router) {
	// Serve either HTTP or HTTPS.
	// In case of HTTPS, all requests to HTTP are redirected.
	//
	// HTTPS served with the aid of Let's Encrypt.

	if secure {

		shost := host
		h := strings.Split(shost, ":")
		if len(h) == 2 {
			shost = h[0]
		}

		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(shost),
			Cache:      autocert.DirCache(".certs"), //folder for storing certificates
		}

		shttp := &http.Server{
			Addr:         shost,
			Handler:      router,
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}
		go serveTLS(shost, shttp)

		s := &http.Server{
			Addr:         host,
			Handler:      http.HandlerFunc(redirect),
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
		}
		log.Println("starting SSL (with redirect from non-SSL)")
		s.ListenAndServe()
	} else {
		log.Println(http.ListenAndServe(host, router))
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

type Plug struct {
	Path    string
	_       chan struct{}
	Symbols map[string]interface{}
}

func inspectPlugin(p *plugin.Plugin) {
	pl := (*Plug)(unsafe.Pointer(p))

	fmt.Printf("Plugin %s exported symbols (%d): \n", pl.Path, len(pl.Symbols))

	for name, pointers := range pl.Symbols {
		fmt.Printf("symbol: %s, pointer: %v, type: %v\n", name, pointers, reflect.TypeOf(pointers))
	}
}
