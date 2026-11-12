package gserver

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/rveen/golib/fn"
	"github.com/rveen/gserver/session"
	"github.com/rveen/ogdl"
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
	Hosts          []string
	Config         *ogdl.Graph
	HostContexts   map[string]*ogdl.Graph
	Context        *ogdl.Graph
	Root           *fn.FNode
	DocRoot        string
	UploadDir      string
	DefaultUser    string
	UserDb         *sql.DB
	MaxSessions    int
	SessionTimeout time.Duration
	Plugins        []string
	Login          login
	ContextService contextService
	DomainConfig   domainConfig
	Templates      map[string]*ogdl.Graph
	Multi          bool
	ContextMu      sync.RWMutex

	// Timeout is the HTTP read and write timeout of Serve (default 10s).
	Timeout time.Duration

	// UserDB is the password check of the login form: "htaccess" or "sql".
	// CheckPassword, when set, replaces it.
	UserDB        string
	CheckPassword func(user, pass string) bool

	// DocFilter post-processes the HTML of DOCV, DocLinksV and docPartV for
	// a logged-in user (Options.DocFilter).
	DocFilter func(html, user string) string

	assets  *Assets
	mu      sync.Mutex // guards server and stopped
	server  *http.Server
	stopped bool
}

func NewWithConfig(host string, config, context *ogdl.Graph) (*Server, error) {

	srv := Server{}

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = host

	// Server configuration
	srv.Config = config

	// Base context
	srv.Context = context

	// Preload templates
	tpls := srv.Config.Get("templates")
	srv.Templates = make(map[string]*ogdl.Graph)
	if tpls.Len() > 0 {
		for _, tpl := range tpls.Out {
			srv.Templates[tpl.ThisString()] = ogdl.NewTemplate(tpl.String())
			log.Printf("Template configured for %s\n", tpl.ThisString())
		}
	}

	srv.registerRemoteFunctions()

	srv.Hosts = append(srv.Hosts, srv.Host)

	// Default Auth
	// srv.Login = LoginService{}

	// Default context builder
	srv.ContextService = nil

	// Session manager
	srv.InitSessions()

	return &srv, nil

}

// InitSessions installs the global session manager. It discards every stored
// session, so it must only be called at startup.
func (srv *Server) InitSessions() {
	srv.MaxSessions = 100000
	srv.SessionTimeout = 30 * time.Minute
	session.Init(session.Options{
		AllowHTTP:    true,
		CookieMaxAge: time.Hour * 24 * 90,
		MaxSessions:  srv.MaxSessions,
	})
}

// SetMaxSessions caps the number of concurrently stored sessions. Sessions are
// only stored for authenticated users, so this bounds concurrent logins; past
// the cap the least recently used session is evicted. 0 means unlimited.
func (srv *Server) SetMaxSessions(n int) {
	srv.MaxSessions = n
	session.SetMaxSessions(n)
}

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
func New(host string) (*Server, error) {

	// Server configuration file (optional)
	config := ogdl.FromFile(".conf/config.ogdl")
	if config == nil {
		return nil, errors.New("missing .conf/config.ogdl file")
	}

	// Base context for templates (optional)
	context := ogdl.FromFile(".conf/context.ogdl")
	if context == nil {
		return nil, errors.New("missing .conf/context.ogdl file")
	}

	return NewWithConfig(host, config, context)
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

	// Server configuration file (optional)
	srv.Config = ogdl.FromFile(".conf/config.ogdl")
	if srv.Config == nil {
		srv.Config = ogdl.New(nil)
	}

	// Base context for templates
	// Each host gets its own
	files, _ := os.ReadDir(".")
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

	srv.registerRemoteFunctions()

	// Default Auth
	// srv.Login = LoginService{}

	// Default context builder
	srv.ContextService = nil

	// Session manager
	srv.InitSessions()

	return &srv, nil
}

// WatchContext watches the given context.ogdl file and reloads srv.Context
// whenever the file is written or replaced. Intended to be run as a goroutine.
func (srv *Server) WatchContext(path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("fsnotify: cannot create watcher:", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		log.Println("fsnotify: cannot watch", path, ":", err)
		return
	}
	log.Println("watching", path)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				newCtx := ogdl.FromFile(path)
				if newCtx == nil {
					log.Println("context reload failed: invalid file", path)
					continue
				}
				// Only the context is swapped, with the built-in functions,
				// remote functions and plugins installed into it first. Re-initialising the session
				// manager here would discard every stored session, dropping
				// the userACL cache and any pending redirect, and would reset
				// SessionTimeout to its default, undoing the -ts flag.
				srv.installInto(newCtx)
				srv.ContextMu.Lock()
				srv.Context = newCtx
				srv.ContextMu.Unlock()
				if srv.ContextService != nil {
					srv.ContextService.GlobalContext(srv)
				}
				log.Println("context reloaded from", path)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("fsnotify error:", err)
		}
	}
}
