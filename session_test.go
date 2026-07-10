package gserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
	"github.com/rveen/session2"
)

// testServer builds the minimum Server needed by getSession(): a context to
// overlay and a Root whose path lands in the R.home node.
func testServer() *Server {
	return &Server{
		Context:        ogdl.New(nil),
		Root:           &fn.FNode{},
		SessionTimeout: 30 * time.Minute,
	}
}

func hasCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return true
		}
	}
	return false
}

// -------------------------------------------------------------------------
// Anonymous requests must not allocate a session
// -------------------------------------------------------------------------

func TestAnonymousRequestCreatesNoSession(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	ctx, sess := getSession(r, w, false, srv)

	if ctx == nil {
		t.Fatal("getSession returned a nil context for a valid host")
	}
	if sess != nil {
		t.Error("anonymous request got a stored session")
	}
	if n := session2.Len(); n != 0 {
		t.Errorf("session2.Len() = %d, want 0", n)
	}
	if hasCookie(w, "sessid") {
		t.Error("anonymous request got a sessid cookie")
	}
}

// An attacker or crawler that ignores Set-Cookie used to create one session per
// request, filling the table. It must now leave no trace at all.
func TestAnonymousFloodCreatesNoSessions(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()
	for i := 0; i < 1000; i++ {
		getSession(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder(), false, srv)
	}
	if n := session2.Len(); n != 0 {
		t.Errorf("session2.Len() = %d after 1000 anonymous requests, want 0", n)
	}
}

// DefaultUser auto-login sets the user scalar but must not allocate a session,
// or an auto-login deployment would allocate one per anonymous hit.
func TestDefaultUserCreatesNoSession(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()
	srv.DefaultUser = "guest"

	w := httptest.NewRecorder()
	ctx, sess := getSession(httptest.NewRequest("GET", "/", nil), w, false, srv)

	if got := ctx.Get("user").String(); got != "guest" {
		t.Errorf("user = %q, want %q", got, "guest")
	}
	if sess != nil || session2.Len() != 0 {
		t.Error("DefaultUser allocated a session")
	}
}

// -------------------------------------------------------------------------
// Authenticated requests do allocate one
// -------------------------------------------------------------------------

func TestAuthenticatedRequestCreatesSession(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()

	// Mint a signed userid cookie the way LoginAdapter does on success.
	cw := httptest.NewRecorder()
	UserCookie().SetValue(cw, []byte("alice"))

	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range cw.Result().Cookies() {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()

	ctx, sess := getSession(r, w, false, srv)

	if sess == nil {
		t.Fatal("authenticated request got no session")
	}
	if n := session2.Len(); n != 1 {
		t.Errorf("session2.Len() = %d, want 1", n)
	}
	if got := ctx.Get("user").String(); got != "alice" {
		t.Errorf("user = %q, want %q", got, "alice")
	}
	if !hasCookie(w, "sessid") {
		t.Error("authenticated request got no sessid cookie")
	}
}

// An identity injected by an upstream Authenticate middleware (bearer token,
// trusted header) also materializes the session.
func TestInjectedUserCreatesSession(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(WithUser(r.Context(), &InjectedUser{UID: "bob", ACL: "rw"}))

	ctx, sess := getSession(r, httptest.NewRecorder(), false, srv)

	if sess == nil {
		t.Fatal("injected identity got no session")
	}
	if got := ctx.Get("user").String(); got != "bob" {
		t.Errorf("user = %q, want %q", got, "bob")
	}
	if got := ctx.Get("userACL").String(); got != "rw" {
		t.Errorf("userACL = %q, want %q", got, "rw")
	}
	if got, _ := sess.Attr("userACL").(string); got != "rw" {
		t.Errorf("session userACL attr = %q, want %q", got, "rw")
	}
}

// -------------------------------------------------------------------------
// Unknown host
// -------------------------------------------------------------------------

func TestUnknownHostReturnsNilContext(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	srv := testServer()
	srv.HostContexts = map[string]*ogdl.Graph{"known.example": ogdl.New(nil)}

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "unknown.example"

	ctx, sess := getSession(r, httptest.NewRecorder(), true, srv)
	if ctx != nil || sess != nil {
		t.Error("unknown host should yield no context and no session")
	}
}

// -------------------------------------------------------------------------
// Redirect stash
// -------------------------------------------------------------------------

func TestRedirectCookieRoundTrip(t *testing.T) {
	w := httptest.NewRecorder()
	SetRedirectCookie(w, "/protected")

	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	if got := RedirectCookieValue(r); got != "/protected" {
		t.Errorf("RedirectCookieValue = %q, want %q", got, "/protected")
	}
}

// Stashing the redirect must not touch the session table: /login is reached
// anonymously, so a session-backed stash would reopen the fill vector.
func TestRedirectStashCreatesNoSession(t *testing.T) {
	session2.Init(session2.Options{AllowHTTP: true, CleanInterval: time.Hour})
	defer session2.Close()

	w := httptest.NewRecorder()
	SetRedirectCookie(w, "/protected")

	if n := session2.Len(); n != 0 {
		t.Errorf("session2.Len() = %d, want 0", n)
	}
}

func TestRedirectCookieValueUnsigned(t *testing.T) {
	// A cookie that was not signed with userCookieKey must be rejected.
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "redirect", Value: "https://evil.example"})
	if got := RedirectCookieValue(r); got != "" {
		t.Errorf("forged redirect cookie accepted: %q", got)
	}
}

func TestSafeRedirect(t *testing.T) {
	cases := map[string]string{
		"/protected":                  "/protected",
		"/a/b?c=d":                    "/a/b?c=d",
		"":                            "/",
		"//evil.example":              "/",
		"https://evil.example":        "/",
		"http://evil.example/x":       "/",
		"javascript:alert(1)":         "/",
		"\\\\evil.example":            "/",
		"/\\evil.example":             "/\\evil.example", // local path, browser-safe
		"///evil.example":             "/",
		"/login?redirect=/x":          "/login?redirect=/x",
		strings.Repeat("/a", 10) + "": strings.Repeat("/a", 10),
	}
	for in, want := range cases {
		if got := safeRedirect(in); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}
