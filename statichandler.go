package gserver

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"
)

// StaticFileHandler returns a handler that processes static files.
//
// if host is true, the hostname is prepended to the path
// if userspace is true, the first element of a path is taken as a user
func (srv *Server) StaticFileHandler(host, userspace, protect bool) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// Needed?
		path := filepath.Clean(r.URL.Path)

		if host {
			path = r.Host + "/" + path
		}

		log.Println("StaticHandler", path)

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
		file := &fd
		err := file.Get(path)

		// Process errors
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if len(file.Content) == 0 {
			http.Error(w, "Zero length file", 500)
			return
		}

		// Set Content-Type (MIME type)
		ext := filepath.Ext(file.Path)
		mimeType := mime.TypeByExtension(ext)
		w.Header().Set("Content-Type", mimeType)

		// Write out
		// Content-Length is set automatically in the Go http lib.

		w.Header().Set("Cache-Control", "public, max-age=7200")
		w.Write(file.Content)
		log.Println("StaticHandler END", path)
	}
}
