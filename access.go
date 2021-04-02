package gserver

import (
	"net/http"
	"strings"

	"github.com/rveen/golib/acl"

	auth "github.com/abbot/go-http-auth"
)

// LoginAdapter sets the "_user" parameter of the request either to "nobody" or to
// the authenticated user name.
func LoginAdapter(srv *Server) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if r.FormValue("Logout") != "" {
				r.Form["_user"] = []string{"nobody"}
				sess := srv.Sessions.Get(r)
				if sess != nil {
					srv.Sessions.Remove(sess, w)
				}

			} else if r.FormValue("Login") != "" {

				// TODO load the file at startup only and after changes.
				// TODO Return an error if the file is not present, do not panic
				secrets := auth.HtpasswdFileProvider(".conf/htpasswd")
				println("htpasswd loaded")

				user := r.FormValue("User")

				if secrets != nil {
					pass := r.FormValue("Password")
					pw := secrets(user, pass)

					if !auth.CheckSecret(pass, pw) {
						http.Redirect(w, r, "/login?redirect="+r.URL.Path, 302)
						return
					}
				}
				r.Form["user"] = []string{user}

				println("user set in r.Form[]:", r.FormValue("user"))

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

func AccessAdapter(config string) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			user := r.FormValue("_user")

			if !checkAccess(user, r.URL.Path) {
				if user == "nobody" {
					http.Redirect(w, r, "/login?message=Restricted area&redirect="+r.URL.Path, 302)
				} else {
					http.Redirect(w, r, "/unauth", 302)
					// http.Error(w, "User "+user+" not authorized", 401)
				}
				return
			}

			h.ServeHTTP(w, r)
		})
	}
	return mw
}

// Access control

var enforcer *acl.ACL

func checkAccess(user, path string) bool {

	if enforcer == nil {
		enforcer, _ = acl.New(".conf/acl.conf")
		if enforcer == nil {
			return true
		}
	}

	// Clean path
	if path == "" {
		path = "/"
	}
	path = strings.ReplaceAll(path, "//", "/")

	return enforcer.Enforce(user, path, "read")
}
