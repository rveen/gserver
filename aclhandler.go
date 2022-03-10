package gserver

import (
	"net/http"
	"strings"

	"github.com/rveen/golib/acl"
)

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
