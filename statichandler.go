package gserver

import (
	"log"
	"net/http"

	"github.com/rveen/golib/fs"

	fr "github.com/DATA-DOG/fastroute"
)

// StaticFileHandler returns a handler that processes static files.
//
func StaticFileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		// These parameters come from the router: "/:user/file/*filepath"
		user := fr.Parameters(r).ByName("user")
		path := "files/" + user + fr.Parameters(r).ByName("filepath")

		log.Printf("StaticFileHandler path %s user %s\n", path, user)

		// Get the file
		file, _ := fs.Get(srv.Root, path, "")

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		buf := file.Content()

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		if len(file.Mime()) > 0 {
			w.Header().Set("Content-Type", file.Mime())
		}

		// Content-Length is set automatically in the Go http lib.

		if len(buf) == 0 {
			log.Println("Zero length file")
			http.Error(w, "Zero length file", 500)
		} else {
			w.Write(buf)
		}
	}

	return http.HandlerFunc(fn)

}
