package gserver

import (
	"net/http/httptest"
	"testing"
)

// A path that is not clean is served as its cleaned form, not redirected:
// behind a proxy that doubles a slash (ProxyPass "/p" "http://host/p/"), the
// redirect would loop.
func TestCleanPathServesUncleanPaths(t *testing.T) {
	srv := newTestSrv(t, setupRoot(t))
	h := cleanPath(srv.Mux())

	for _, c := range []struct {
		path     string
		code     int
		location string
		body     string
	}{
		{"/onlyroot.htm", 200, "", "ROOT-OK"},
		{"//onlyroot.htm", 200, "", "ROOT-OK"},
		{"/./onlyroot.htm", 200, "", "ROOT-OK"},
		{"/prot/../onlyroot.htm", 200, "", "ROOT-OK"},
		{"/../onlyroot.htm", 200, "", "ROOT-OK"},
		// The trailing slash survives cleaning, so a directory stays one:
		// /prot// is /prot/ and goes to login, not to a slash redirect.
		{"/prot//", 302, "/login?redirect=/prot/", ""},
		// A directory without its slash still gets the one 301 that adds it.
		{"//prot", 302, "/login?redirect=/prot", ""},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.URL.Path = c.path
		r.RequestURI = c.path
		h.ServeHTTP(w, r)

		if w.Code != c.code || w.Header().Get("Location") != c.location {
			t.Errorf("%s: got %d Location %q, want %d %q", c.path, w.Code, w.Header().Get("Location"), c.code, c.location)
		}
		if c.body != "" && w.Body.String() != c.body {
			t.Errorf("%s: got body %q, want %q", c.path, w.Body.String(), c.body)
		}
	}
}

// Without cleanPath, ServeMux redirects; this is what looped on galileo.
func TestMuxAloneRedirectsUncleanPaths(t *testing.T) {
	srv := newTestSrv(t, setupRoot(t))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.URL.Path = "//onlyroot.htm"
	srv.Mux().ServeHTTP(w, r)
	if w.Code != 301 || w.Header().Get("Location") != "/onlyroot.htm" {
		t.Errorf("got %d %q, want 301 /onlyroot.htm", w.Code, w.Header().Get("Location"))
	}
}
