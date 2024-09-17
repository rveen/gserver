package gserver

import (
	"database/sql"
	"log"
	"net/http"

	auth "github.com/abbot/go-http-auth"
	_ "modernc.org/sqlite"
)

// LoginAdapter handles "Login" and "Logout"
//
// Login: sets r.Form["user"] to the authenticated user name.
// Logout: removes the session
// Other: do nothing
func (srv *Server) LoginAdapter(host bool, userdb string) func(http.Handler) http.Handler {

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			userCookie := UserCookie()

			if r.FormValue("Logout") != "" {
				sess := srv.Sessions.Get(r)
				if sess != nil {
					srv.Sessions.Remove(sess, w)
					userCookie.SetValue(w, []byte("-"))
				}

			} else if r.FormValue("Login") != "" {

				user := r.FormValue("User")
				pass := r.FormValue("Password")

				if !validateUser(user, pass, userdb) {
					sess := srv.Sessions.Get(r)
					if sess != nil {
						srv.Sessions.Remove(sess, w)
						userCookie.SetValue(w, []byte("-"))
					}
					http.Redirect(w, r, "/login?redirect="+r.URL.Path, 302)
					return
				}

				rq := ConvertRequest(r, w, host, srv)
				r.Form["user"] = []string{user}
				rq.User = user
				rq.Context.Set("user", user)

				// Set user cookie.
				// This is the way to communicate the user to the request.
				// In request.Convert() the session's 'user' is set to
				// the value of this cookie.

				userCookie.SetValue(w, []byte(user))

				if rdir := r.FormValue("redirect"); rdir != "" {
					http.Redirect(w, r, rdir, 302)
					return
				}
			}

			h.ServeHTTP(w, r)
		})
	}
	return mw
}

func validateUser(user, pass, userdb string) bool {

	switch userdb {

	case "htaccess":

		secrets := auth.HtpasswdFileProvider(".conf/htpasswd")
		log.Println("htpasswd loaded")

		if secrets != nil {
			pw := secrets(user, pass)
			return auth.CheckSecret(pass, pw)
		}

	case "sqlite":

		// Concurrent open is safe ??
		db, err := sql.Open("sqlite", ".conf/users.db")
		defer db.Close()

		if err != nil {
			return false
		}
		row := db.QueryRow("select name,passwd from contacts where email='" + user + "'")
		var name, passwd string
		row.Scan(&name, &passwd)
		return passwd == pass

	}
	return false
}
