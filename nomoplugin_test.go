package gserver

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// The Nomo worksheet viewer, registered as main.go registers it.
	_ "github.com/rveen/golib/formats/nomo/plugin"
)

func nomoRoot(t *testing.T) string {
	t.Helper()
	root := setupRoot(t)
	sheet := []byte("r = 5 cm\nh = 12 cm\nV = r^2*h/3\n")
	for _, p := range []string{"cone.nomo", "prot/cone.nomo"} {
		if err := os.WriteFile(filepath.Join(root, p), sheet, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestNomoIsServedTypeset(t *testing.T) {
	h := newTestSrv(t, nomoRoot(t)).DynamicHandler(false)

	w := do(h, "/cone.nomo")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	for _, want := range []string{"<title>cone</title>", "<math", "/.nomo/stix-two-math-subset.woff2"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}

	// Revalidation answers without rendering.
	r := httptest.NewRequest("GET", "/cone.nomo", nil)
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w2 := httptest.NewRecorder()
	h(w2, r)
	if w2.Code != 304 {
		t.Errorf("revalidation: got %d, want 304", w2.Code)
	}
}

func TestNomoRawIsTheSource(t *testing.T) {
	h := newTestSrv(t, nomoRoot(t)).DynamicHandler(false)
	if w := do(h, "/cone.nomo?m=raw"); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "r = 5 cm") {
		t.Errorf("raw: got %d %q", w.Code, w.Body.String())
	}
}

func TestNomoFontIsServed(t *testing.T) {
	h := newTestSrv(t, nomoRoot(t)).DynamicHandler(false)
	w := do(h, "/.nomo/stix-two-math-subset.woff2")
	if w.Code != 200 || w.Header().Get("Content-Type") != "font/woff2" || !strings.HasPrefix(w.Body.String(), "wOF2") {
		t.Errorf("font: got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := do(h, "/.nomo/OFL.txt"); w.Code != 200 {
		t.Errorf("licence: got %d", w.Code)
	}
}

func TestNomoRespectsProtectedPaths(t *testing.T) {
	h := newTestSrv(t, nomoRoot(t)).DynamicHandler(false)
	if w := do(h, "/prot/cone.nomo"); w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Errorf("protected: got %d %q, want a redirect to /login", w.Code, w.Header().Get("Location"))
	}
}

func TestMissingNomoIs404(t *testing.T) {
	h := newTestSrv(t, nomoRoot(t)).DynamicHandler(false)
	if w := do(h, "/absent.nomo"); w.Code != 404 {
		t.Errorf("missing: got %d, want 404", w.Code)
	}
}
