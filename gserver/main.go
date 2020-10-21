// Copyright 2017-2019, Rolf Veen and contributors.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO See https://medium.com/@matryer/how-i-write-go-http-services-after-seven-years-37c208122831

// Gserver is a web server.
//
// Summary of features
//
// Gserver is a web server with the following features:
//
//  - Any path of the form /[user]/file/* is served as static (StaticHandler) and
//    converted to /files/user/*
//  - Any other path is handled by Handler as follows.
//  - Path elements of the form _n (n = number) are taken as revision numbers
//  - Path elements of the form _t (t != number) are taken as variables
//  - Extensions of files are optional (if the file name is unique)
//  - index.* (if found) is returned for paths that point to directories.
//  - OGDL templates are processed
//  - Markdown is processed
//  - The root directory must be a standard directory. Below there can be versioned
//    repositories
//  - The path can continue into data files and documents (markdown)
//
// Authentication and sessions
//
// - htpasswd, SVN Auth, ACL
//
// Templates
//
//
// TODO
//
// - relative paths (for images, etc)
//
// - math notebook / wiki / forms
//
// - resumable file uploader
//
package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	"github.com/rveen/golib/fs"
	"github.com/rveen/gserver"
	"github.com/rveen/gserver/context"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/justinas/alice"
)

func main() {

	// Bind flags to non-pointer variables. Easier later.

	var logging, verbose, hosts bool
	var host, secureHost string
	var timeout, sessionTimeout int

	flag.BoolVar(&logging, "log", true, "turn logging ON/off")
	flag.BoolVar(&hosts, "m", false, "enable multiple hosts (path on disk are affected")
	flag.BoolVar(&verbose, "v", false, "turn periodic status message on/OFF")
	flag.StringVar(&host, "H", ":80", "set host:port")
	flag.StringVar(&secureHost, "S", "", "set secure_host:port")
	flag.IntVar(&timeout, "t", 10, "set http(s) timeout (seconds)")
	flag.IntVar(&sessionTimeout, "ts", 30, "set session timeout (minutes)")

	flag.Parse()

	secure := false
	if secureHost != "" {
		host = secureHost
		secure = true
	}

	srv, err := gserver.New()
	if err != nil {
		log.Println(err.Error())
		return
	}

	// srv.Login = gserver.LoginService{}
	srv.ContextService = context.ContextService{}
	// srv.DomainConfig = gserver.DomainConfig{}
	//	srv.Root.GetConfig = gserver.DomainConfig.GetConfig

	// Middleware chains
	// staticHandler := alice.New(gserver.LoginAdapter(srv), gserver.AccessAdapter("bla")).Then(gserver.StaticFileHandler(srv))
	staticHandler := gserver.StaticFileHandler(srv, false)
	staticUserHandler := alice.New(gserver.LoginAdapter(srv), gserver.AccessAdapter("bla")).Then(gserver.StaticFileHandler(srv, true))
	dynamicHandler := alice.New(gserver.LoginAdapter(srv), gserver.AccessAdapter("bla")).Then(gserver.FileHandler(srv))

	// Router
	// (see github.com/DATA-DOG/fastroute for all the possibilities of this router).

	router := fr.RouterFunc(func(req *http.Request) http.Handler {
		return fr.Chain(fr.New("/static/*filepath", staticHandler), fr.New("/:user/file/*filepath", staticUserHandler), fr.New("/*filepath", dynamicHandler))
	})

	log.Println("gserver starting, ", runtime.NumCPU(), "procs")

	if logging == false {
		println("further logging disabled!")
		log.SetOutput(ioutil.Discard)
	}

	if verbose {
		go printStatus(srv)
	}

	// Overwrite the original file handler with this one
	srv.Root = fs.New(srv.DocRoot)

	srv.Serve(host, secure, timeout, router)
}

func printStatus(srv *gserver.Server) {

	for {
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		log.Println("Mem (use/reserved)", m.HeapInuse, "/", m.Alloc)

		time.Sleep(5 * time.Second)
	}
}
