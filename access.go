package gserver

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rveen/golib/acl"

	auth "github.com/abbot/go-http-auth"
	"github.com/icza/session"
)

// LoginAdapter sets the "user" parameter of the request either to "nobody" or to
// the authenticated user name.
// If the request has a session, the "user" parameter in that session is updated
// if there is a change.
func LoginAdapter(srv *Server) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			sess := srv.Sessions.Get(r)

			if r.FormValue("Logout") != "" {
				// setCookie(w, "")
				r.Form["user"] = []string{"nobody"}
				if sess != nil {
					srv.Sessions.Remove(sess, w)
				}
				log.Println("user(a)", r.Form["user"])
				h.ServeHTTP(w, r)

				return
			}

			if r.FormValue("Login") == "" {

				// Is there a session ?
				if sess == nil {
					// println("user reset to nobody:", r.FormValue("user"))

					if r.Form["user"] == nil {
						r.Form["user"] = []string{"nobody"}
					}
				} else {
					// log.Println("user(b2)", sess.Attr("user").(string))
					r.Form["user"] = []string{sess.Attr("user").(string)}
				}
				// log.Println("user(b)", r.Form["user"])
				h.ServeHTTP(w, r)
				return

			}

			// TODO: load the file at startup only and after changes.
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

			// Create a new session if not already done
			if sess == nil {
				sess = session.NewSessionOptions(&session.SessOptions{Timeout: 90 * 24 * time.Hour})
				srv.Sessions.Add(sess, w)
			}
			sess.SetAttr("user", user)
			r.Form["user"] = []string{user}

			println("user set in r.Form[]:", r.FormValue("user"))
			println("user set in session:", sess.Attr("user").(string))

			if rdir := r.FormValue("redirect"); rdir != "" {
				if rdir == "_user" {
					rdir = "/" + user
				}
				http.Redirect(w, r, rdir, 302)
				return
			}
			log.Println("user(c)", r.Form["user"])

			h.ServeHTTP(w, r)
		})
	}
	return mw
}

func AccessAdapter(config string) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			user := r.FormValue("user")

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

/*
const cookieName = "guser"

func AuthByCookie(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	log.Println("AuthByCookie succeded", cookie.Value)
	return cookie.Value
}

func setCookie(w http.ResponseWriter, user string) {

	if user == "" {
		cookie := http.Cookie{Name: cookieName, Value: "", MaxAge: -1}
		http.SetCookie(w, &cookie)
		//cookie2 := http.Cookie{Name: "sessid", Value: "", MaxAge: -1}
		//http.SetCookie(w, &cookie2)

	} else {
		cookie := http.Cookie{Name: cookieName, Value: user, MaxAge: 100000000}
		http.SetCookie(w, &cookie)
	}
	log.Println("SetCookie", cookieName, user)
}

*/

// Access control

var enforcer *acl.ACL

func checkAccess(user, path string) bool {

	if path == "/favicon.ico" {
		return true
	}

	if strings.HasPrefix(path, "/static/") {
		return true
	}

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
