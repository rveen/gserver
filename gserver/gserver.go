// Copyright 2017, Rolf Veen and contributors.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// gserver is a web server adapted to serve OGDL templates besides static content.
//
// Features:
// - The file extension of files in the web/ area is optional
// - Trailing slash and index files detection
// - Login and Logout detection in forms.
// - Uploaded files go to files/user/<folder>/*
//
// How are templates identified ?
//
// - Router with * and ?
// - Anything not in /static/
//
package main

import (
	"flag"
	"gserver"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	fr "github.com/DATA-DOG/fastroute"
)

func main() {

	// Flags
	// - set logging ON/off
	// - set host
	// - set secure host
	// - set credential files
	// - set web root
	// - set timeout

	var logging, verbose bool
	var host, secureHost string
	var timeout, sessionTimeout int

	flag.BoolVar(&logging, "log", true, "Turn logging ON/off")
	flag.BoolVar(&verbose, "v", false, "Turn status message on/OFF")
	flag.StringVar(&host, "H", ":80", "Set host:port")
	flag.StringVar(&secureHost, "S", "", "Set secure_host:port")
	flag.IntVar(&timeout, "t", 10, "Set http(s) timeout (seconds)")
	flag.IntVar(&sessionTimeout, "ts", 30, "Set session timeout (minutes)")

	flag.Parse()

	// Set up a Server{} structure

	srv, err := gserver.New()
	if err != nil {
		log.Println(err.Error())
		return
	}

	if verbose {
		go printStatus(srv)
	}

	// Router
	// (see github.com/DATA-DOG/fastroute for all the possibilities of this router).

	router := fr.RouterFunc(func(req *http.Request) http.Handler {
		return fr.Chain(fr.New("/:user/file/*filepath", gserver.StaticFileHandler(srv)), fr.New("/*filepath", gserver.FileHandler(srv)))
	})

	log.Println("Server starting, ", runtime.NumCPU(), "procs")

	if logging == false {
		println("further logging disabled!")
		log.SetOutput(ioutil.Discard)
	}

	// Serve either http or https. In case of https, all requests to http are
	// redirected to https.

	if len(secureHost) != 0 {
		shttp := &http.Server{
			Addr:         secureHost,
			Handler:      router,
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
		}
		go serveTLS(shttp)

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
		http.StatusTemporaryRedirect)
}

func serveTLS(srv *http.Server) {
	log.Println(srv.ListenAndServeTLS(".conf/cert.pem", ".conf/key.pem"))
}

func printStatus(srv *gserver.Server) {

	for {
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		log.Println("Mem (use/reserved)", m.HeapInuse, "/", m.Alloc)

		time.Sleep(5 * time.Second)
	}
}
