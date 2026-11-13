// Copyright 2017-2026, Rolf Veen.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

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
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rveen/gserver/v3"

	// Context dependencies: each blank import self-registers a named object
	// into the template context via ctxreg. Add or remove a dependency by
	// editing this list only.
	_ "github.com/rveen/gserver/v3/context/plugins/etoolsctx"
	_ "github.com/rveen/gserver/v3/context/plugins/filesctx"
	_ "github.com/rveen/gserver/v3/context/plugins/gosqlctx"
	_ "github.com/rveen/gserver/v3/context/plugins/htmlctx"
	_ "github.com/rveen/gserver/v3/context/plugins/isolationctx"
	_ "github.com/rveen/gserver/v3/context/plugins/stringsctx"

	// Optional request interceptors: each blank import self-registers an HTTP
	// interceptor (golib/fn/httphook) run before normal file resolution. Remove
	// the line to drop the dependency entirely.
	_ "github.com/rveen/golib/formats/altium/plugin"
	_ "github.com/rveen/golib/formats/nomo/plugin"
)

func main() {

	// Command line, then the `server` section of the site's config file for
	// whatever the command line did not set.
	var o gserver.Options
	o.BindFlags(flag.CommandLine)
	flag.Parse()

	if err := o.ApplyConfig(filepath.Join(o.Root, ".conf/config.ogdl")); err != nil {
		fail(err)
	}

	srv, err := gserver.Open(o)
	if err != nil {
		fail(err)
	}
	go srv.WatchContext(".conf/context.ogdl")

	log.Println("gserver starting, ", runtime.NumCPU(), "procs")

	// Serve until the listener fails or a SIGTERM / SIGINT arrives (systemctl
	// stop, Ctrl-C); in the latter case let running requests finish.
	if err := gserver.Run(srv, srv.Mux()); err != nil {
		fail(err)
	}
}

// fail reports a fatal error and exits with status 1, so that systemd (or
// any supervisor) sees the failure. It writes to stderr even with -log=false.
func fail(v any) {
	fmt.Fprintln(os.Stderr, "gserver:", v)
	os.Exit(1)
}
