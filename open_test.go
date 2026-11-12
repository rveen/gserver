package gserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rveen/gserver/context/ctxreg"
	"github.com/rveen/gserver/session"
	"github.com/rveen/ogdl"
)

const (
	testConfig    = "protected\n  /prot\nplug\n  name from-config\n"
	testContext   = "title Hello\n"
	testTemplates = "# page wrappers\ndocument\n  <main>$path.content</main>\n"
)

// writeSite writes files (path: content) under dir.
func writeSite(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// openSite opens a server on a fresh site directory. Open changes the working
// directory; t.Chdir restores it afterwards.
func openSite(t *testing.T, o Options, files map[string]string) *Server {
	t.Helper()
	t.Chdir(t.TempDir())
	site := t.TempDir()
	writeSite(t, site, files)
	o.Root = site
	srv, err := Open(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	return srv
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func standardSite() map[string]string {
	return map[string]string{
		".conf/config.ogdl":    testConfig,
		".conf/context.ogdl":   testContext,
		".conf/templates.ogdl": testTemplates,
		"index.htm":            "title=$title plug=$testplug",
		"doc.md":               "# Heading\n\nText.\n",
		"prot/secret.htm":      "SECRET",
		"static/x.css":         "CSS",
		"files/a.txt":          "FILES",
		"file/b.txt":           "FILE",
	}
}

func init() {
	// A plugin built from the server config.
	ctxreg.Register("testplug", func(cfg *ogdl.Graph) any {
		return cfg.Get("plug.name").String()
	})
}

func TestOpenFromDirectory(t *testing.T) {
	srv := openSite(t, Options{}, standardSite())
	mux := srv.Mux()

	for _, c := range []struct{ path, want string }{
		{"/", "title=Hello plug=from-config"},
		{"/static/x.css", "CSS"},
		{"/files/a.txt", "FILES"},
		{"/file/b.txt", "FILE"},
	} {
		if w := get(t, mux, c.path); w.Code != 200 || w.Body.String() != c.want {
			t.Errorf("GET %s: got %d %q, want 200 %q", c.path, w.Code, w.Body.String(), c.want)
		}
	}

	// Markdown goes through the "document" wrapper of templates.ogdl.
	if w := get(t, mux, "/doc"); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<main>") || !strings.Contains(w.Body.String(), "Heading") {
		t.Errorf("GET /doc: got %d %q, want the document wrapped in <main>", w.Code, w.Body.String())
	}

	// config.ogdl's protected paths apply.
	if w := get(t, mux, "/prot/secret.htm"); w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Errorf("GET /prot/secret.htm: got %d -> %q, want 302 -> /login", w.Code, w.Header().Get("Location"))
	}

	// Paths that are not clean are redirected, not served.
	if w := get(t, mux, "/a//b"); w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/a/b" {
		t.Errorf("GET /a//b: got %d -> %q, want 301 -> /a/b", w.Code, w.Header().Get("Location"))
	}

	// The built-ins, KF's included, are in the context.
	for _, name := range []string{"T", "DOC", "DOCV", "DocLinksV", "docPart", "docPartV", "docDataPath", "docDataHtml"} {
		if srv.Context.Node(name) == nil {
			t.Errorf("context has no %s", name)
		}
	}

	if fi, err := os.Stat(".tmp"); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("upload directory .tmp: %v %v, want a searchable directory", fi, err)
	}
}

func TestOpenRequiresConfigAndContext(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := Open(Options{}); err == nil || !strings.Contains(err.Error(), "config.ogdl") {
		t.Errorf("got %v, want an error naming config.ogdl", err)
	}
}

func TestOpenRejectsBadOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := Open(Options{UserDB: "ldap"}); err == nil || !strings.Contains(err.Error(), "ldap") {
		t.Errorf("UserDB ldap: got %v, want an error", err)
	}
	if _, err := Open(Options{Multi: true, Assets: &Assets{}}); err == nil {
		t.Error("Multi with Assets: got nil, want an error")
	}
}

func TestOpenFromEmbeddedFiles(t *testing.T) {
	assets := &Assets{
		Conf: fstest.MapFS{
			".conf/config.ogdl":    {Data: []byte(testConfig)},
			".conf/context.ogdl":   {Data: []byte("title Embedded\n")},
			".conf/templates.ogdl": {Data: []byte(testTemplates)},
		},
		Dynamic: fstest.MapFS{"index.htm": {Data: []byte("title=$title")}},
		Static:  fstest.MapFS{"static/x.css": {Data: []byte("EMBEDDED-CSS")}},
	}
	// The site directory holds only what is not embedded.
	srv := openSite(t, Options{Assets: assets}, map[string]string{"disk.htm": "DISK", "file/b.txt": "UPLOADED"})
	mux := srv.Mux()

	for _, c := range []struct{ path, want string }{
		{"/", "title=Embedded"},
		{"/static/x.css", "EMBEDDED-CSS"},
		{"/disk.htm", "DISK"}, // a miss in Dynamic falls back to the site directory
		{"/file/b.txt", "UPLOADED"},
	} {
		if w := get(t, mux, c.path); w.Code != 200 || w.Body.String() != c.want {
			t.Errorf("GET %s: got %d %q, want 200 %q", c.path, w.Code, w.Body.String(), c.want)
		}
	}
}

func postLogin(t *testing.T, h http.Handler, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"Login": {"1"}, "User": {user}, "Password": {pass}}
	r := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCheckPassword(t *testing.T) {
	check := func(user, pass string) bool { return user == "ana" && pass == "right" }
	srv := openSite(t, Options{CheckPassword: check}, standardSite())
	mux := srv.Mux()

	w := postLogin(t, mux, "ana", "right")
	if w.Code != http.StatusSeeOther || !hasCookie(w, "userid") {
		t.Errorf("right password: got %d, cookie %v; want 303 with a userid cookie", w.Code, hasCookie(w, "userid"))
	}

	w = postLogin(t, mux, "ana", "wrong")
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Errorf("wrong password: got %d -> %q, want 302 -> /login", w.Code, w.Header().Get("Location"))
	}
}

func TestShutdownBeforeServe(t *testing.T) {
	srv := &Server{Host: "127.0.0.1:0"}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(http.NotFoundHandler()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve after Shutdown: %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve after Shutdown did not return")
	}
}

func TestServeAndShutdown(t *testing.T) {
	ln, err := net0{}.addr()
	if err != nil {
		t.Skip("no free port:", err)
	}
	srv := &Server{Host: ln}
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "up") }))
	}()

	// Wait until it answers.
	var body string
	for i := 0; i < 100 && body != "up"; i++ {
		if resp, err := http.Get("http://" + ln + "/"); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if body != "up" {
		t.Fatal("server did not answer")
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("Serve returned %v after Shutdown, want nil", err)
	}
}

// net0 finds a free loopback port.
type net0 struct{}

func (net0) addr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}
