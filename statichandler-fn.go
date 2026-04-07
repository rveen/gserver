package gserver

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rveen/golib/fn"
)

// StaticFileHandler returns a handler that processes static files.
//
// if host is true, the hostname is prepended to the path
func (srv *Server) StaticFileHandlerFn(host bool, fs *fn.FNode) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		path := r.URL.Path

		if host {
			path = r.Host + "/" + path
		}

		log.Println("StaticHandler-fn", path, r.RemoteAddr)

		// Get the file. Make a copy of the struct!
		fd := *fs
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
			w.Header().Set("Cache-Control", "public, max-age=36000")
			http.ServeContent(w, r, filepath.Base(file.Path), stat.ModTime(), f)
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
		w.Header().Set("Cache-Control", "public, max-age=36000")
		w.Write(file.Content)
		// log.Println("StaticHandler-fn END", path)
	}
}
