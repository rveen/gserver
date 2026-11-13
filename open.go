package gserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl/v2"
)

// Open sets up a server as described by o: it changes to the site directory,
// sets up logging, reads .conf/config.ogdl, .conf/context.ogdl and
// .conf/templates.ogdl (from Root, or from Assets.Conf), sets up sessions,
// installs the template context (the built-in functions and every plugin
// registered with ctxreg) and creates the upload directory.
//
// The server is not yet listening: pass its Mux, or a router built from its
// Handlers, to Serve or Run.
func Open(o Options) (*Server, error) {

	o = o.withDefaults()

	switch o.UserDB {
	case "htaccess", "sql":
	case "sqlite":
		o.UserDB = "sql"
	default:
		return nil, fmt.Errorf("unknown user db %q: want htaccess or sql", o.UserDB)
	}

	if o.Multi && o.Assets != nil {
		return nil, errors.New("embedded assets are not supported with multiple hosts")
	}

	// Every path the server uses is relative to the site directory.
	if o.Root != "" {
		if err := os.Chdir(o.Root); err != nil {
			return nil, err
		}
	}

	if o.LogFile != "" {
		f, err := os.OpenFile(o.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		log.SetOutput(f)
	} else if os.Getenv("JOURNAL_STREAM") != "" {
		// journald adds its own timestamps; other flags a build set stay.
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime | log.Lmicroseconds))
	}

	var srv *Server
	var err error

	if o.Multi {
		srv, err = NewMulti()
	} else {
		var conf fs.FS = os.DirFS(".")
		if o.Assets != nil && o.Assets.Conf != nil {
			conf = o.Assets.Conf
		}
		var config, ctx *ogdl.Node
		if config, err = readOgdl(conf, ".conf/config.ogdl"); err != nil {
			return nil, err
		}
		if ctx, err = readOgdl(conf, ".conf/context.ogdl"); err != nil {
			return nil, err
		}
		srv, err = NewWithConfig(o.Host, config, ctx)
		if err == nil {
			err = srv.loadTemplates(conf, ".conf/templates.ogdl")
		}
	}
	if err != nil {
		return nil, err
	}

	srv.Host = o.Host
	srv.Timeout = o.Timeout
	srv.SessionTimeout = o.SessionTimeout
	if o.MaxSessions > 0 {
		srv.SetMaxSessions(o.MaxSessions)
	}
	srv.UserDB = o.UserDB
	srv.CheckPassword = o.CheckPassword
	srv.DocFilter = o.DocFilter
	srv.assets = o.Assets
	srv.Root = fn.New(srv.DocRoot)

	srv.InstallContext()

	CreateUploadDir()

	if o.NoLog {
		log.Println("further logging disabled")
		log.SetOutput(io.Discard)
	}

	return srv, nil
}

// readOgdl reads a required OGDL file from fsys.
func readOgdl(fsys fs.FS, name string) (*ogdl.Node, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("missing %s file: %w", name, err)
	}
	g, err := ogdl.ParseV1(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return g, nil
}

// loadTemplates adds the page templates of an optional templates.ogdl to
// srv.Templates, one per top-level node.
//
// They are a separate file because an OGDL block scalar ends the node it
// belongs to: written as children of a `templates` node in config.ogdl, only
// the first survives.
func (srv *Server) loadTemplates(fsys fs.FS, name string) error {

	b, err := fs.ReadFile(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	g := ogdl.FromBytes(b)
	if g == nil {
		return nil
	}

	for _, n := range g.Out {
		tname := n.ThisString()
		// Comment lines parse as nodes named "#".
		if tname == "" || tname[0] == '#' {
			continue
		}
		t, err := ogdl.ParseTemplateV1(n.String())
		if err != nil {
			return fmt.Errorf("%s: template %s: %w", name, tname, err)
		}
		srv.Templates[tname] = t
		log.Printf("Template configured for %s\n", tname)
	}
	return nil
}

// Handlers are the HTTP handlers a server is made of, for a build that wires
// its own router. Mux shows the default wiring.
type Handlers struct {
	Static    http.Handler                    // /static/, /favicon.ico: from Assets.Static if given, else Root
	Protected http.Handler                    // static files from Root, 401 without a logged-in user (KF's /file/)
	Files     http.Handler                    // /files/: http.FileServer, streaming and Range-safe
	Dynamic   http.Handler                    // everything else: templates, Markdown, data
	Login     func(http.Handler) http.Handler // login and logout form handling, around Dynamic
}

// Handlers returns the server's handlers, reading from Options.Assets where
// they were given.
func (srv *Server) Handlers() Handlers {

	var static, dynamic *fn.FNode
	if a := srv.assets; a != nil {
		if a.Static != nil {
			static = fn.NewFS(a.Static)
		}
		if a.Dynamic != nil {
			dynamic = fn.NewFS(a.Dynamic)
		}
	}

	return Handlers{
		Static:    srv.staticHandler(srv.Multi, false, static),
		Protected: srv.staticHandler(srv.Multi, true, nil),
		Files:     FileHandler(),
		Dynamic:   srv.dynamicHandler(srv.Multi, dynamic),
		Login:     srv.LoginAdapter(srv.Multi, srv.UserDB),
	}
}

// Mux returns the default routing on a ServeMux:
//
//	/favicon.ico, /static/  Static
//	/file/                  static files from Root, also with Assets
//	/files/                 Files
//	/                       Login(Dynamic)
//
// ServeMux answers a path that is not clean (//, /./, /../) with a 301 to its
// cleaned form.
func (srv *Server) Mux() *http.ServeMux {
	h := srv.Handlers()
	mux := http.NewServeMux()
	mux.Handle("/favicon.ico", h.Static)
	mux.Handle("/files/", h.Files)
	mux.Handle("/static/", h.Static)
	// Uploaded files are on disk even when the site is embedded.
	mux.Handle("/file/", srv.staticHandler(srv.Multi, false, nil))
	mux.Handle("/", h.Login(h.Dynamic))
	return mux
}

// Serve serves plain HTTP on srv.Host until the listener fails or Shutdown is
// called, after which it returns nil. gserver does not do TLS: put it behind a
// proxy (Caddy, Apache httpd, nginx) that terminates HTTPS.
func (srv *Server) Serve(h http.Handler) error {

	timeout := srv.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if srv.Host == "" {
		srv.Host = DefaultHost
	}

	server := &http.Server{
		Addr:              srv.Host,
		Handler:           h,
		ReadTimeout:       timeout,
		WriteTimeout:      timeout,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: timeout,
	}
	srv.mu.Lock()
	if srv.stopped {
		// Shutdown came first (Run got a signal before the server started).
		srv.mu.Unlock()
		return nil
	}
	srv.server = server
	srv.mu.Unlock()

	log.Println("starting, host:", srv.Host)
	return serveErr(server.ListenAndServe())
}

// Shutdown stops the server, letting the requests in progress finish until
// ctx is done. A stopped Server does not serve again: open a new one.
func (srv *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server")
	srv.mu.Lock()
	srv.stopped = true
	server := srv.server
	srv.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

// Run serves h until the listener fails or a SIGTERM or SIGINT arrives
// (systemctl stop, Ctrl-C); in the latter case it lets the requests in
// progress finish, and returns nil.
func Run(srv *Server, h http.Handler) error {

	done := make(chan error, 1)
	go func() { done <- srv.Serve(h) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)

	select {
	case err := <-done:
		return err
	case s := <-sig:
		log.Println("received", s)
		if err := srv.Shutdown(context.Background()); err != nil {
			return err
		}
		return <-done
	}
}

// serveErr filters out the error returned after a regular Shutdown.
func serveErr(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
