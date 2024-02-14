package gserver

import (
	"log"
	"net/http"

	auth "github.com/abbot/go-http-auth"
)

// LoginAdapter handles "Login" and "Logout"
//
// Login: sets r.Form["_user"] to the authenticated user name.
// Logout: removes the session
// Other: do nothing
func (srv *Server) LoginAdapterHtpasswd(host bool) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if r.FormValue("Logout") != "" {
				sess := srv.Sessions.Get(r)
				if sess != nil {
					srv.Sessions.Remove(sess, w)
				}

			} else if r.FormValue("Login") != "" {

				// TODO load the file at startup only and after changes.
				// TODO Return an error if the file is not present, do not panic
				secrets := auth.HtpasswdFileProvider(".conf/htpasswd")
				log.Println("htpasswd loaded")

				user := r.FormValue("User")

				if secrets != nil {
					pass := r.FormValue("Password")
					pw := secrets(user, pass)

					if !auth.CheckSecret(pass, pw) {
						http.Redirect(w, r, "/login?redirect="+r.URL.Path, 302)
						return
					}
				}

				rq := ConvertRequest(r, w, host, srv)
				r.Form["user"] = []string{user}
				rq.User = user
				rq.Context.Set("user", user)

				log.Println("user set in r.Form[]:", user)

				if rdir := r.FormValue("redirect"); rdir != "" {
					if rdir == "_user" {
						rdir = "/" + user
					}
					http.Redirect(w, r, rdir, 302)
					return
				}
			}

			h.ServeHTTP(w, r)
		})
	}
	return mw
}
