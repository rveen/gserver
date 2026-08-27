package gserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
)

// dirRoot builds a tree exercising every case the redirect has to tell apart:
// a directory with an index, a directory without one, a file, a document that
// only resolves by extension guessing, and a protected directory.
func dirRoot(t *testing.T) string {
	t.Helper()
	d := t.TempDir()

	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(d, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("index.html", "<!doctype html><h1>site</h1>")
	write("app/index.html", `<!doctype html><script type="module" src="bundle.js"></script>`)
	write("app/bundle.js", "export const answer = 42;\n")
	write("doc.md", "# Doc\n\n## cap1\n\ntext\n")
	write("plain/a.txt", "a\n")
	write("plain/b.txt", "b\n")
	write("prot/index.html", "<!doctype html>secret")
	if err := os.MkdirAll(filepath.Join(d, "plain", "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	return d + "/"
}

func dirSrv(t *testing.T, tpls string) *Server {
	t.Helper()
	cfg := ogdl.FromString("protected\n  /prot\n" + tpls)
	srv, err := NewWithConfig(":0", cfg, ogdl.FromString("dummy 1"))
	if err != nil {
		t.Fatal(err)
	}
	srv.Root = fn.New(dirRoot(t))
	return srv
}

func doDyn(t *testing.T, srv *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.DynamicHandler(false).ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

// A URL naming a directory is answered with a 301 to the same URL plus a
// slash. Without it, every relative reference in the page -- and every entry
// of a directory listing -- resolves against the parent.
func TestDirectoryURLRedirects(t *testing.T) {
	srv := dirSrv(t, "")

	for _, path := range []string{"/app", "/plain", "/plain/sub"} {
		w := doDyn(t, srv, "GET", path)
		if w.Code != http.StatusMovedPermanently {
			t.Errorf("GET %s = %d; want 301", path, w.Code)
		}
		if got := w.Header().Get("Location"); got != path+"/" {
			t.Errorf("GET %s -> Location %q; want %q", path, got, path+"/")
		}
	}
}

func TestDirectoryRedirectKeepsTheQuery(t *testing.T) {
	w := doDyn(t, dirSrv(t, ""), "GET", "/app?m=raw&x=1")
	if got := w.Header().Get("Location"); got != "/app/?m=raw&x=1" {
		t.Errorf("Location = %q; want /app/?m=raw&x=1", got)
	}
}

// The redirect must terminate. A second one would be worse than the bug.
func TestTrailingSlashIsNotRedirectedAgain(t *testing.T) {
	w := doDyn(t, dirSrv(t, ""), "GET", "/app/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /app/ = %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bundle.js") {
		t.Errorf("GET /app/ served %q; want app/index.html", w.Body.String())
	}
}

// Anything that is not literally a directory is left alone -- including the
// paths Get resolves by guessing an extension or by continuing into a
// document, which have no directory to redirect to.
func TestNonDirectoriesAreNotRedirected(t *testing.T) {
	srv := dirSrv(t, "")

	for _, path := range []string{"/", "/app/bundle.js", "/doc", "/doc/cap1", "/index.html"} {
		w := doDyn(t, srv, "GET", path)
		if w.Code == http.StatusMovedPermanently {
			t.Errorf("GET %s = 301 -> %q; want no redirect", path, w.Header().Get("Location"))
		}
	}
}

// A 301 permits a client to reissue a POST as a GET, dropping the form body.
// Templates gate writes on R.method, so that would turn a submission into a
// read without anyone noticing.
func TestPostToDirectoryIsNotRedirected(t *testing.T) {
	w := doDyn(t, dirSrv(t, ""), "POST", "/app")
	if w.Code == http.StatusMovedPermanently {
		t.Errorf("POST /app = 301; want no redirect")
	}
}

// The redirect runs after the auth check, so the difference between a 301 and
// a login redirect never tells an anonymous caller that a directory exists.
func TestProtectedDirectoryGoesToLoginNotToASlash(t *testing.T) {
	w := doDyn(t, dirSrv(t, ""), "GET", "/prot")
	if w.Code != http.StatusFound {
		t.Fatalf("GET /prot = %d; want 302 to /login", w.Code)
	}
	if got := w.Header().Get("Location"); !strings.HasPrefix(got, "/login") {
		t.Errorf("Location = %q; want /login...", got)
	}
}

// Fault 1, end to end: a missing file no longer comes back as the directory's
// index under a 200.
func TestMissingAssetIs404NotTheIndex(t *testing.T) {
	srv := dirSrv(t, "")

	for _, path := range []string{"/app/missing.js", "/app/nope.css", "/app/x.wasm"} {
		w := doDyn(t, srv, "GET", path)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d (%q); want 404", path, w.Code, w.Body.String())
		}
	}
}

// A directory with no index.* is answered with its listing, and with no "dir"
// template configured that listing is the built-in one.
func TestDirectoryWithoutIndexIsListed(t *testing.T) {
	w := doDyn(t, dirSrv(t, ""), "GET", "/plain/")

	if w.Code != http.StatusOK {
		t.Fatalf("GET /plain/ = %d (%q); want 200", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q; want text/html", ct)
	}

	body := w.Body.String()
	for _, want := range []string{`href="a.txt"`, `href="b.txt"`, `href="sub/"`, `href="../"`} {
		if !strings.Contains(body, want) {
			t.Errorf("listing has no %s:\n%s", want, body)
		}
	}
}

// Entries are linked relatively, so the listing is correct only at a URL
// ending in a slash. That is what the redirect guarantees.
func TestRootListingHasNoParentLink(t *testing.T) {
	srv := dirSrv(t, "")
	if err := os.Remove(filepath.Join(strings.TrimSuffix(srv.Root.Root, "/"), "index.html")); err != nil {
		t.Fatal(err)
	}

	w := doDyn(t, srv, "GET", "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d; want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), `href="../"`) {
		t.Error("root listing links above the document root")
	}
}

// A configured "dir" template always wins; the built-in listing is only a
// fallback for sites that have none.
func TestConfiguredDirTemplateWins(t *testing.T) {
	srv := dirSrv(t, "\ntemplates\n  dir \"CUSTOM LISTING\"\n")

	w := doDyn(t, srv, "GET", "/plain/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /plain/ = %d; want 200", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, "CUSTOM LISTING") {
		t.Errorf("body = %q; want the configured template", got)
	}
}

// Names are attacker-controlled: a file can be created with markup in its name.
func TestListingEscapesEntryNames(t *testing.T) {
	srv := dirSrv(t, "")
	evil := `<img src=x onerror=alert(1)>.txt`
	if err := os.WriteFile(filepath.Join(strings.TrimSuffix(srv.Root.Root, "/"), "plain", evil), []byte("x"), 0644); err != nil {
		t.Skipf("filesystem rejects the name: %v", err)
	}

	body := doDyn(t, srv, "GET", "/plain/").Body.String()
	if strings.Contains(body, "<img src=x") {
		t.Errorf("entry name is not escaped:\n%s", body)
	}
	if !strings.Contains(body, "&lt;img") {
		t.Errorf("escaped name missing from listing:\n%s", body)
	}
}

// Path variables (/_user matches any element) are not followable URLs, so
// linking them would only produce dead entries.
func TestListingOmitsPathVariables(t *testing.T) {
	srv := dirSrv(t, "")
	if err := os.MkdirAll(filepath.Join(strings.TrimSuffix(srv.Root.Root, "/"), "plain", "_user"), 0755); err != nil {
		t.Fatal(err)
	}

	if body := doDyn(t, srv, "GET", "/plain/").Body.String(); strings.Contains(body, "_user") {
		t.Errorf("listing links a path variable:\n%s", body)
	}
}

// A zero-byte file is a legitimate 200, not an "Empty content" 500.
func TestEmptyFileIsServedNotAn500(t *testing.T) {
	srv := dirSrv(t, "")
	if err := os.WriteFile(filepath.Join(strings.TrimSuffix(srv.Root.Root, "/"), "plain", "empty.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	w := doDyn(t, srv, "GET", "/plain/empty.txt")
	if w.Code != http.StatusOK {
		t.Errorf("GET /plain/empty.txt = %d (%q); want 200", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q; want empty", w.Body.String())
	}
}
