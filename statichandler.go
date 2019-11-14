package gserver

import (
	"log"
	"net/http"

	"github.com/rveen/golib/fs/sysfs"

	fr "github.com/DATA-DOG/fastroute"
)

// StaticUserFileHandler returns a handler that processes static user files.
//
func StaticUserFileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		// These parameters come from the router: "/:user/file/*filepath"
		user := fr.Parameters(r).ByName("user")
		path := "files/" + user + fr.Parameters(r).ByName("filepath")

		log.Printf("StaticUserFileHandler path %s user %s\n", path, user)

		// Get the file
		file, _ := sysfs.Get(srv.Root, path, "")

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		buf := file.Content()

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		// TODO check that we get the correct Mime here!
		if len(file.Type()) > 0 {
			w.Header().Set("Content-Type", file.Type())
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

// StaticFileHandler returns a handler that processes static files.
//
func StaticFileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		path := r.URL.Path

		// log.Printf("StaticFileHandler path %s\n", path)

		// Get the file
		file, _ := sysfs.Get(srv.Root, path, "")

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		buf := file.Content()

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		// TODO check that we get the correct Mime here!
		if len(file.Type()) > 0 {
			w.Header().Set("Content-Type", file.Type())
		}

		log.Printf("StaticFileHandler path %s mime %s\n", path, file.Mime())

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
