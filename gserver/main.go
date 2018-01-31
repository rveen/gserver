// Copyright 2017-2018, Rolf Veen and contributors.
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
	"crypto/tls"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"time"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/rveen/gserver"
	"golang.org/x/crypto/acme/autocert"
)

func main() {

	// Bind flags to non-pointer variables. Easier later.

	var logging, verbose bool
	var host, secureHost string
	var timeout, sessionTimeout int

	flag.BoolVar(&logging, "log", true, "turn logging ON/off")
	flag.BoolVar(&verbose, "v", false, "turn periodic status message on/OFF")
	flag.StringVar(&host, "H", ":80", "set host:port")
	flag.StringVar(&secureHost, "S", "", "set secure_host:port")
	flag.IntVar(&timeout, "t", 10, "set http(s) timeout (seconds)")
	flag.IntVar(&sessionTimeout, "ts", 30, "set session timeout (minutes)")

	flag.Parse()

	// Set up a Server{} structure

	srv, err := gserver.New()
	if err != nil {
		log.Println(err.Error())
		return
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

	if verbose {
		go printStatus(srv)
	}

	// Serve either HTTP or HTTPS.
	// In case of HTTPS, all requests to HTTP are redirected.
	//
	// HTTPS served with the aid of Let's Encrypt.

	if len(secureHost) != 0 {

		theHost := secureHost
		log.Println("secure host:", secureHost)
		h := strings.Split(secureHost, ":")
		if len(h) == 2 {
			theHost = h[0]
		}
		log.Println("secure domain:", theHost)

		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(theHost),
			Cache:      autocert.DirCache(".certs"), //folder for storing certificates
		}

		shttp := &http.Server{
			Addr:         secureHost,
			Handler:      router,
			ReadTimeout:  time.Second * time.Duration(timeout),
			WriteTimeout: time.Second * time.Duration(timeout),
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}
		go serveTLS(theHost, shttp)

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

// TODO Check https://github.com/nhooyr/redirecthttp

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

func printStatus(srv *gserver.Server) {

	for {
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		log.Println("Mem (use/reserved)", m.HeapInuse, "/", m.Alloc)

		time.Sleep(5 * time.Second)
	}
}
