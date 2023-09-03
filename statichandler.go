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
//
func (srv *Server) StaticFileHandler(host, userspace bool) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// Needed?
		path := filepath.Clean(r.URL.Path)

		if host {
			path = r.Host + "/" + path
		}

		log.Println("StaticHandler", path)

		/*
			for name, values := range r.Header {
				// Loop over all values for the name.
				for _, value := range values {
					log.Println(" - header:", name, value)
				}
			}

			// Referrer based file access limitation (future?)
			ref := r.Header["Referer"]
			if ref == nil || !strings.Contains(ref[0], "/item/") {
				http.Error(w, "file cannot be accessed directly", 500)
				return
			}
		*/

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
	}
}
