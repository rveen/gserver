package gserver

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"

	fr "github.com/DATA-DOG/fastroute"
)

// StaticFileHandler returns a handler that processes static files.
//
// if host is true, the hostname is prepended to the path
// if userspace is true, the first element of a path is taken as a user
//
func StaticFileHandler(srv *Server, host, userspace bool) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		var path string

		// Adjust the path
		if userspace {
			// These parameters come from the router: "/:user/file/*filepath"
			user := fr.Parameters(r).ByName("user")
			path = "files/" + user + fr.Parameters(r).ByName("filepath")
			path = filepath.Clean(path)
		} else {
			path = filepath.Clean(r.URL.Path)
		}

		if host {
			path = r.Host + "/" + path
		}

		log.Println("StaticHandler", path)

		// Get the file
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

		w.Header().Set("Cache-Control", "public, max-age=36000")
		w.Write(file.Content)
	}

	return http.HandlerFunc(fn)

}
