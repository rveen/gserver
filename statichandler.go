package gserver

import (
	"log"
	"net/http"
	"path/filepath"

	fr "github.com/DATA-DOG/fastroute"
	"github.com/icza/session"
)

// FileHandler processes all paths that exist in the file system starting
// from the root directory, whether they are static files or templates or markdown.
//
// NOTE This handler is a final one. If the path doesn't exist, it returns 'Not found'
// NOTE This handler needs context information (access to Server{})
// NOTE See https://github.com/bpowers/seshcookie
//
func StaticFileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		user := fr.Parameters(r).ByName("user")
		path := "files/" + user + fr.Parameters(r).ByName("filepath")

		log.Printf("StaticFileHandler path %s user %s\n", path, user)

		// Get a session, whether or not the user has logged in
		sess := srv.Sessions.Get(r)
		if sess == nil {
			sess = session.NewSession()
			sess.SetAttr("user", "nobody")
			srv.Sessions.Add(sess, w)
		} else if r.FormValue("Logout") != "" {
			log.Println("Logout requested")
			srv.Sessions.Remove(sess, w)
			sess = session.NewSession()
			sess.SetAttr("user", "nobody")
			srv.Sessions.Add(sess, w)
		}

		// Get the file
		path = filepath.Clean(path)
		file, _, _ := srv.Root.Get(path)

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		// log.Println("FileHandler", url, file.Type)

		buf := file.Content

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		if len(file.Mime) > 0 {
			w.Header().Set("Content-Type", file.Mime)
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
