package gserver

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	uu "net/url"

	"github.com/go-ldap/ldap/v3"

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

				}
				DeleteUserCookie(w)

			} else if r.FormValue("Login") != "" {

				user := r.FormValue("User")
				pass := r.FormValue("Password")

				if !validateUser(user, pass, userdb, srv) {
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

func validateUser(user, pass, userdb string, srv *Server) bool {

	switch userdb {

	case "htaccess":

		secrets := auth.HtpasswdFileProvider("../htpasswd")
		log.Println("htpasswd loaded")

		if secrets != nil {
			pw := secrets(user, pass)
			return auth.CheckSecret(pass, pw)
		}

	case "sqlite":

		if srv.UserDb == nil {
			srv.UserDb, _ = sql.Open("sqlite", "../users.db")
		}
		fallthrough

	case "sql":

		if srv.UserDb == nil {
			log.Println("srv.UserDb is nil")
			return false
		}

		row := srv.UserDb.QueryRow("select password from users where user='" + user + "'")
		var name, passwd string
		row.Scan(&name, &passwd)
		return passwd == pass

	case "ldap":

		c := srv.Config.Get("ldap")
		host := c.Node("server").String()
		buser := c.Node("user").String()
		bpass := c.Node("password").String()
		dn := c.Node("basedn").String()

		l, err := ldap.Dial("tcp", host)
		if err != nil {
			log.Println(err)
			return false
		}
		defer l.Close()

		// Reconnect with TLS
		/*
			err = l.StartTLS(&tls.Config{InsecureSkipVerify: true})
			if err != nil {
				log.Println(err)
				return false
			}
		*/
		// First bind with a read only user
		err = l.Bind(buser, bpass)
		if err != nil {
			log.Println(err)
			return false
		}

		// Search for the given username
		searchRequest := ldap.NewSearchRequest(
			dn,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf("(&(objectClass=organizationalPerson)(uid=%s))", user),
			[]string{"dn"},
			nil,
		)

		sr, err := l.Search(searchRequest)
		if err != nil {
			log.Println(err)
			return false
		}

		if len(sr.Entries) != 1 {
			log.Println(err)
			return false
		}

		userdn := sr.Entries[0].DN

		// Bind as the user to verify their password
		err = l.Bind(userdn, pass)
		if err != nil {
			log.Println(err)
			return false
		}
		return true

	}

	return false
}
