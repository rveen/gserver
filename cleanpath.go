package gserver

import (
	"net/http"
	"path"
	"strings"
)

// cleanPath serves a request whose path is not clean (//, /./, /../) as if
// its path were the cleaned one, instead of letting ServeMux answer it with a
// 301 to the cleaned path.
//
// The 301 loops behind a reverse proxy that adds the extra slash on every
// pass: Apache's `ProxyPass "/projects" "http://localhost:82/projects/"`
// forwards /projects/rolf as /projects//rolf, the browser follows the 301 to
// /projects/rolf, and the proxy doubles the slash again. gserver v1, with its
// own router, accepted such paths, so sites behind proxies configured like
// that worked; this keeps them working.
//
// A trailing slash is kept, since it tells a directory from a file. Cleaning
// cannot climb above the root, so /../x is served as /x.
func cleanPath(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/") {
			np := path.Clean(p)
			if strings.HasSuffix(p, "/") && np != "/" {
				np += "/"
			}
			if np != p {
				r2 := new(http.Request)
				*r2 = *r
				u := *r.URL
				u.Path = np
				u.RawPath = ""
				r2.URL = &u
				r = r2
			}
		}
		h.ServeHTTP(w, r)
	})
}
