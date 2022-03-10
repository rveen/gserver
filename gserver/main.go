// Copyright 2017-2022, Rolf Veen.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO See https://pace.dev/blog/2018/05/09/how-I-write-http-services-after-eight-years.html

// Gserver is a web server.
//
// Summary of features
//
//  - Any path of the form /[user]/file/* is served as static (StaticHandler) and
//    converted to /files/user/*
//  - Any other path is handled by Handler as follows.
//  - Path elements of the form @rev are taken as revisions.
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
	"net/http/pprof"
	"runtime"

	"github.com/rveen/golib/fn"
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

	var srv *gserver.Server
	var err error

	if !hosts {
		srv, err = gserver.New(host)
	} else {
		srv, err = gserver.NewMulti()
	}
	if err != nil {
		log.Println(err.Error())
		return
	}

	// srv.Login = gserver.LoginService{}
	srv.ContextService = context.ContextService{}

	// Middleware chains
	staticHandler := srv.StaticFileHandler(hosts, false)
	dynamicHandler := alice.New(srv.LoginAdapter(), gserver.AccessAdapter("bla")).Then(srv.DynamicHandler(hosts))

	router := fr.RouterFunc(func(req *http.Request) http.Handler {
		return fr.Chain(fr.New("/favicon.ico", staticHandler),

			fr.New("/debug/pprof/*filepath", http.HandlerFunc(pprof.Index)),

			fr.New("/static/*filepath" /*staticEmbedded*/, staticHandler),
			fr.New("/file/*filepath", staticHandler),
			fr.New("/*filepath", dynamicHandler))
	})

	log.Println("gserver starting, ", runtime.NumCPU(), "procs")

	if logging == false {
		println("further logging disabled!")
		log.SetOutput(ioutil.Discard)
	}

	// Overwrite the original file handler with this one
	srv.Root = fn.New(srv.DocRoot)

	srv.Serve(secure, timeout, router)
}
