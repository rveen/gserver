// Copyright 2017-2026, Rolf Veen.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Gserver is a web server.
//
// Summary of features
//
//   - Any path of the form /file/* is served as static (StaticHandler)
//   - Any other path is handled by DynHandler as follows.
//   - Path elements of the form @rev are taken as revisions.
//   - Path elements of the form _t (t != number) are taken as variables
//   - Extensions of files are optional (if the file name is unique)
//   - index.* (if found) is returned for paths that point to directories.
//   - OGDL templates are processed
//   - Markdown is processed
//   - The root directory must be a standard directory. Below there can be versioned
//     repositories
//   - The path can continue into data files and documents (markdown)
//
// # TODO
// - relative paths (for images, etc)
// - math notebook / wiki / forms
// - resumable file uploader
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	//	"net/http/pprof"
	"runtime"

	"github.com/rveen/golib/fn"
	"github.com/rveen/gserver"
	"github.com/rveen/gserver/context"
	"github.com/rveen/ogdl"

	// Context dependencies: each blank import self-registers a named object
	// into the template context via ctxreg. Add or remove a dependency by
	// editing this list only — context.go and the gserver package stay untouched.
	_ "github.com/rveen/gserver/context/plugins/etoolsctx"
	_ "github.com/rveen/gserver/context/plugins/filesctx"
	_ "github.com/rveen/gserver/context/plugins/gosqlctx"
	_ "github.com/rveen/gserver/context/plugins/htmlctx"
	_ "github.com/rveen/gserver/context/plugins/isolationctx"
	_ "github.com/rveen/gserver/context/plugins/stringsctx"

	// Optional request interceptors: each blank import self-registers an HTTP
	// interceptor (golib/fn/httphook) run before normal file resolution. Remove
	// the line to drop the dependency entirely.
	_ "github.com/rveen/golib/formats/altium/plugin"
	_ "github.com/rveen/golib/formats/nomo/plugin"

	// TODO Change these for go-chi
	fr "github.com/DATA-DOG/fastroute"
	"github.com/justinas/alice"
)

// configKeys maps keys of the 'server' section in .conf/config.ogdl to
// command line flags. Flags given on the command line take precedence.
//
//	server
//	  host :8080
//	  email someone@example.com
var configKeys = map[string]string{
	"log":             "log",
	"multi":           "m",
	"verbose":         "v",
	"host":            "H",
	"secure_host":     "S",
	"timeout":         "t",
	"session_timeout": "ts",
	"max_sessions":    "ms",
	"userdb":          "userdb",
	"email":           "email",
	"certs":           "certs",
}

func main() {

	// Bind flags to non-pointer variables. Easier later.

	var logging, verbose, hosts bool
	var host, secureHost, userdb, email, root, certs string
	var timeout, sessionTimeout, maxSessions int

	// flag.BoolVar(&logging, "static", false, "serve all files static") --> disables extension discovery

	flag.BoolVar(&logging, "log", true, "turn logging ON/off")
	flag.BoolVar(&hosts, "m", false, "enable multiple hosts (path on disk are affected")
	flag.BoolVar(&verbose, "v", false, "turn periodic status message on/OFF")
	flag.StringVar(&host, "H", ":80", "set host:port")
	flag.StringVar(&secureHost, "S", "", "set secure_host:port")
	flag.IntVar(&timeout, "t", 10, "set http(s) timeout (seconds)")
	flag.IntVar(&sessionTimeout, "ts", 30, "set session timeout (minutes)")
	flag.IntVar(&maxSessions, "ms", 0, "max concurrent logged-in sessions (0 = leave default)")
	flag.StringVar(&userdb, "userdb", "htaccess", "user db: sqlite or htaccess (default)")
	flag.StringVar(&email, "email", "", "email for Let's Encrypt SSL")
	flag.StringVar(&root, "root", "", "site directory (default: current directory)")
	flag.StringVar(&certs, "certs", "", "directory for Let's Encrypt certificates (default: certmagic's, under $HOME)")

	flag.Parse()

	// All server paths are relative to the site directory
	if root != "" {
		if err := os.Chdir(root); err != nil {
			fail(err)
		}
	}

	if err := applyConfig(".conf/config.ogdl"); err != nil {
		fail(err)
	}

	// journald adds its own timestamps
	if os.Getenv("JOURNAL_STREAM") != "" {
		log.SetFlags(0)
	}

	secure := false
	if secureHost != "" {
		host = secureHost
		secure = true
	}

	if secure && email == "" && host != ":443" && host != "localhost:443" {
		fail("secure host needs an email (for Let's Encrypt)")
	}

	var srv *gserver.Server
	var err error

	if !hosts {
		srv, err = gserver.New(host)
	} else {
		srv, err = gserver.NewMulti()
	}
	if err != nil {
		fail(err)
	}

	if sessionTimeout > 0 {
		srv.SessionTimeout = time.Duration(sessionTimeout) * time.Minute
	}

	if maxSessions > 0 {
		srv.SetMaxSessions(maxSessions)
	}

	srv.CertDir = certs

	// srv.Login = gserver.LoginService{}
	srv.ContextService = context.ContextService{}
	srv.ContextService.GlobalContext(srv)
	go srv.WatchContext(".conf/context.ogdl")

	// Middleware chains
	staticHandler := srv.StaticFileHandler(hosts, false, false)
	dynamicHandler := alice.New(srv.LoginAdapter(hosts, userdb) /*, gserver.AccessAdapter("bla")*/).Then(srv.DynamicHandler(hosts))
	fileHandler := gserver.FileHandler()

	router := fr.RouterFunc(func(req *http.Request) http.Handler {
		return fr.Chain(fr.New("/favicon.ico", staticHandler),
			// fr.New("/.well-known/*filepath", staticHandler), // Letsencript ACME
			// fr.New("/debug/pprof/*filepath", http.HandlerFunc(pprof.Index)),
			fr.New("/files/*filepath", fileHandler),
			fr.New("/static/*filepath", staticHandler),
			fr.New("/file/*filepath", staticHandler),
			fr.New("/*filepath", dynamicHandler))
	})

	log.Println("gserver starting, ", runtime.NumCPU(), "procs")

	if logging == false {
		println("further logging disabled!")
		log.SetOutput(io.Discard)
	}

	// Overwrite the original file handler with this one
	srv.Root = fn.New(srv.DocRoot)

	// Serve until the listener fails or a SIGTERM / SIGINT arrives (systemctl
	// stop, Ctrl-C); in the latter case let running requests finish.
	done := make(chan error, 1)
	go func() { done <- srv.Serve(secure, timeout, router, email) }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err = <-done:
	case s := <-sig:
		log.Println("received", s)
		srv.Shutdown()
		err = <-done
	}
	if err != nil {
		fail(err)
	}
}

// applyConfig sets flags not given on the command line from the 'server'
// section of the config file, if present.
func applyConfig(file string) error {

	cfg := ogdl.FromFile(file).Get("server")
	if cfg == nil {
		return nil
	}

	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })

	for _, n := range cfg.Out {
		key := n.ThisString()
		name, ok := configKeys[key]
		if !ok {
			return fmt.Errorf("%s: unknown key server.%s", file, key)
		}
		if given[name] {
			continue
		}
		if err := flag.Set(name, n.String()); err != nil {
			return fmt.Errorf("%s: server.%s: %v", file, key, err)
		}
	}
	return nil
}

// fail reports a fatal error and exits with status 1, so that systemd (or
// any supervisor) sees the failure. It writes to stderr even with -log=false.
func fail(v any) {
	fmt.Fprintln(os.Stderr, "gserver:", v)
	os.Exit(1)
}
