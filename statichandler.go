package gserver

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rveen/golib/fn"
)

// StaticFileHandler returns a handler that processes static files from srv.Root.
//
// if host is true, the hostname is prepended to the path
// if userspace is true, the first element of a path is taken as a user
func (srv *Server) StaticFileHandler(host, userspace, protect bool) http.HandlerFunc {
	return srv.staticHandler(host, protect, nil)
}

// StaticFileHandlerFn returns a handler that processes static files from fs.
//
// if host is true, the hostname is prepended to the path
//
// Deprecated: retained for github.com/trukeio/gserver.
func (srv *Server) StaticFileHandlerFn(host bool, fs *fn.FNode) http.HandlerFunc {
	return srv.staticHandler(host, false, fs)
}

func (srv *Server) staticHandler(host, protect bool, fs *fn.FNode) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// Needed?
		// path := filepath.Clean(r.URL.Path) : Windows shit
		path := r.URL.Path

		if host {
			path = r.Host + "/" + path
		}

		// Check that a valid user has been set
		if protect {
			u := UserCookieValue(r)
			if (u == "" || u == "nobody") && srv.DefaultUser == "" {
				http.Error(w, "Need to log in to access this content", 401)
				return
			}
		}

		// Get the file. We make a copy of the struct!
		fd := *srv.Root
		if fs != nil {
			fd = *fs
		}
		file := &fd

		// Phase 1: resolve path without reading content
		err := file.GetMeta(path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Phase 2a: native-FS plain file — stream directly via http.ServeContent
		if file.Type == "file" && file.RootFs == nil {
			f, err := os.Open(file.Path)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer f.Close()
			stat, err := f.Stat()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			ext := filepath.Ext(file.Path)
			w.Header().Set("Content-Type", mime.TypeByExtension(ext))
			w.Header().Set("Cache-Control", "public, max-age=7200")
			http.ServeContent(w, r, filepath.Base(file.Path), stat.ModTime(), f)
			log.Printf("StaticHandler #0 %s %s %t\n", path, r.RemoteAddr, protect)
			return
		}

		// Phase 2b: embedded FS or document/data — full read
		if err := file.Get(path); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if len(file.Content) == 0 {
			http.Error(w, "Zero length file", 500)
			return
		}
		ext := filepath.Ext(file.Path)
		w.Header().Set("Content-Type", mime.TypeByExtension(ext))
		w.Header().Set("Cache-Control", "public, max-age=7200")
		w.Write(file.Content)
		log.Printf("StaticHandler #0 %s %s %t\n", path, r.RemoteAddr, protect)
	}
}
