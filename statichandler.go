package gserver

import (
	"log"
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
		file, err := srv.Root.Get(path, "")

		// Process errors
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}
		if len(file.Content) == 0 {
			http.Error(w, "Zero length file", 500)
			return
		}

		// Set Content-Type (MIME type)
		if file.Type != "" {
			w.Header().Set("Content-Type", file.Type)
		}

		// Write out
		// Content-Length is set automatically in the Go http lib.
		w.Write(file.Content)
	}

	return http.HandlerFunc(fn)

}
