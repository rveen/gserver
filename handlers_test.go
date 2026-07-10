package gserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
)

func newTestSrv(t *testing.T, root string) *Server {
	t.Helper()
	cfg := ogdl.FromString("protected\n  /prot\n")
	ctx := ogdl.FromString("dummy 1")
	srv, err := NewWithConfig(":0", cfg, ctx)
	if err != nil {
		t.Fatal(err)
	}
	srv.Root = fn.New(root)
	return srv
}

func setupRoot(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "onlyroot.htm"), []byte("ROOT-OK"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, "prot"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "prot", "secret.htm"), []byte("SECRET"), 0644); err != nil {
		t.Fatal(err)
	}
	return d + "/"
}

func do(h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestFnBranch(t *testing.T) {
	root := setupRoot(t)
	srv := newTestSrv(t, root)
	efs := fn.NewFS(fstest.MapFS{
		"hello.htm": &fstest.MapFile{Data: []byte("EMBED-OK")},
	})

	h := srv.DynamicHandlerFn(false, efs)

	// 1. served from the embedded fs
	if w := do(h, "/hello.htm"); w.Code != 200 || w.Body.String() != "EMBED-OK" {
		t.Errorf("fs hit: got %d %q, want 200 EMBED-OK", w.Code, w.Body.String())
	}

	// 2. miss in fs -> falls back to srv.Root
	if w := do(h, "/onlyroot.htm"); w.Code != 200 || w.Body.String() != "ROOT-OK" {
		t.Errorf("fallback: got %d %q, want 200 ROOT-OK", w.Code, w.Body.String())
	}

	// 3. miss in both -> 404
	if w := do(h, "/nowhere.htm"); w.Code != 404 {
		t.Errorf("miss: got %d, want 404", w.Code)
	}

	// 4. PRESERVED ASYMMETRY: fs != nil skips checkPath, so a protected path
	//    is served anonymously instead of redirecting to /login.
	if w := do(h, "/prot/secret.htm"); w.Code != 200 || w.Body.String() != "SECRET" {
		t.Errorf("fs protected: got %d %q, want 200 SECRET (checkPath must NOT run)", w.Code, w.Body.String())
	}
}

func TestNilFsBranchEnforcesCheckPath(t *testing.T) {
	root := setupRoot(t)
	srv := newTestSrv(t, root)

	h := srv.DynamicHandler(false)

	// fs == nil -> checkPath runs -> anonymous protected path redirects to /login
	w := do(h, "/prot/secret.htm")
	if w.Code != 302 {
		t.Errorf("nil-fs protected: got %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login?redirect=/prot/secret.htm" {
		t.Errorf("Location = %q", loc)
	}

	// unprotected path still served from srv.Root
	if w := do(h, "/onlyroot.htm"); w.Code != 200 || w.Body.String() != "ROOT-OK" {
		t.Errorf("nil-fs normal: got %d %q, want 200 ROOT-OK", w.Code, w.Body.String())
	}
}

func TestStaticFnBranch(t *testing.T) {
	root := setupRoot(t)
	srv := newTestSrv(t, root)
	efs := fn.NewFS(fstest.MapFS{
		"a.css": &fstest.MapFile{Data: []byte("body{}")},
	})

	// embedded fs served, unified max-age
	w := do(srv.StaticFileHandlerFn(false, efs), "/a.css")
	if w.Code != 200 || w.Body.String() != "body{}" {
		t.Errorf("static fs: got %d %q", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=7200" {
		t.Errorf("Cache-Control = %q, want max-age=7200", cc)
	}

	// srv.Root path still works, and protect=true still gates
	w = do(srv.StaticFileHandler(false, false, true), "/onlyroot.htm")
	if w.Code != 401 {
		t.Errorf("protect=true anonymous: got %d, want 401", w.Code)
	}
}
