package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A site configured for the removed HTTPS support must fail at startup with an
// explanation, not a bare "unknown key".
func TestApplyConfigRemovedKeys(t *testing.T) {
	for _, key := range []string{"secure_host", "email", "certs"} {
		file := filepath.Join(t.TempDir(), "config.ogdl")
		if err := os.WriteFile(file, []byte("server\n  "+key+" x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := applyConfig(file)
		if err == nil || !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("server.%s: got %v, want a 'no longer supported' error", key, err)
		}
	}
}

func TestApplyConfigUnknownKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.ogdl")
	if err := os.WriteFile(file, []byte("server\n  colour blue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := applyConfig(file)
	if err == nil || !strings.Contains(err.Error(), "unknown key server.colour") {
		t.Errorf("got %v, want an 'unknown key' error", err)
	}
}

func TestRoutes(t *testing.T) {
	h := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, name) })
	}
	mux := routes(h("static"), h("files"), h("dynamic"))

	for _, c := range []struct{ path, want string }{
		{"/", "dynamic"},
		{"/favicon.ico", "static"},
		{"/favicon.ico/x", "dynamic"},
		{"/files/a/b.pdf", "files"},
		{"/static/x.css", "static"},
		{"/file/u/a.txt", "static"},
		{"/filex", "dynamic"},
		{"/docs/cap1", "dynamic"},
		{"/_user/1", "dynamic"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != 200 || rec.Body.String() != c.want {
			t.Errorf("GET %s: got %d %q, want 200 %q", c.path, rec.Code, rec.Body.String(), c.want)
		}
	}

	// Paths that are not clean are redirected, not served.
	for path, loc := range map[string]string{"/a//b": "/a/b", "/files/../etc/passwd": "/etc/passwd", "/files": "/files/"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != loc {
			t.Errorf("GET %s: got %d -> %q, want 301 -> %q", path, rec.Code, rec.Header().Get("Location"), loc)
		}
	}
}
