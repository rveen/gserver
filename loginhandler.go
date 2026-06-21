package gserver

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"net/http"
	uu "net/url"

	auth "github.com/abbot/go-http-auth"
)

// LoginAdapter handles "Login" and "Logout"
//
// Login: sets r.Form["user"] to the authenticated user name.
// Logout: removes the session
// Other: do nothing
func (srv *Server) LoginAdapter(host bool, userdb string) func(http.Handler) http.Handler {

	log.Printf("LoginAdapter, userdb: %s\n", userdb)

	mw := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			userCookie := UserCookie()

			if r.FormValue("Logout") != "" {
				sess := srv.Sessions.Get(r)
				if sess != nil {
					srv.Sessions.Remove(sess, w)
				}
				DeleteUserCookie(w)
				http.Redirect(w, r, "/login", 302)
				return

			} else if r.FormValue("Login") != "" {

				user := r.FormValue("User")
				pass := r.FormValue("Password")

				ok, acl := validateUser(user, pass, userdb, srv)
				if !ok {
					sess := srv.Sessions.Get(r)
					if sess != nil {
						srv.Sessions.Remove(sess, w)
					}
					DeleteUserCookie(w)
					http.Redirect(w, r, "/login?redirect="+r.URL.Path, 302)
					return
				}

				rq := ConvertRequest(r, w, host, srv)
				r.Form["user"] = []string{user}
				rq.User = user
				rq.Context.Set("user", user)
				rq.Context.Set("userACL", acl)
				r.URL.User = uu.User(user)

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

func GetACL(user string, srv *Server) string {

	if srv.UserDb == nil {
		return ""
	}

	row := srv.UserDb.QueryRow("select acl from users where user='" + user + "'")

	var acl string
	row.Scan(&acl)

	return acl
}

func validateUser(user, pass, userdb string, srv *Server) (bool, string) {

	log.Printf("user %s pass %s userdb %s\n", user, pass, userdb)

	switch userdb {

	case "htaccess":

		secrets := auth.HtpasswdFileProvider("../htpasswd")
		// secrets := auth.HtpasswdFileProvider(".conf/htpasswd")
		log.Println("htpasswd loaded")

		if secrets != nil {
			pw := secrets(user, pass)
			return auth.CheckSecret(pass, pw), ""
		}

	case "sql":

		if srv.UserDb == nil {
			log.Println("srv.UserDb is nil")
			return false, ""
		}

		row := srv.UserDb.QueryRow("select passwd,acl from users where user='" + user + "'")
		var passwd, acl string
		err := row.Scan(&passwd, &acl)

		log.Printf("passwd %s acl %s err %v\n", passwd, acl, err)

		if err != nil {
			log.Printf("validateUser error: %s\n", err.Error())
		}

		hash := md5.Sum([]byte(pass))
		pass = hex.EncodeToString(hash[:])

		log.Printf("passwd %s <> pass %s\n", passwd, pass)

		return passwd == pass, acl

	}

	return false, ""
}
