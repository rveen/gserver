// Copyright 2017-2018, Rolf Veen and contributors.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/rveen/gserver"
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
