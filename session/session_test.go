// Copyright 2026, Rolf Veen.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// requestWith returns a request carrying the cookie set in rec, if any.
func requestWith(rec *httptest.ResponseRecorder) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func cookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cs := rec.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cs))
	}
	return cs[0]
}

func TestNewSession(t *testing.T) {
	a, b := NewSession(), NewSession()
	if a.ID() == b.ID() || len(a.ID()) != 24 {
		t.Errorf("IDs %q, %q: want distinct, 24 characters", a.ID(), b.ID())
	}
	if a.Timeout() != 30*time.Minute {
		t.Errorf("default timeout %v", a.Timeout())
	}
	if !a.Accessed().Equal(a.Created()) {
		t.Error("a new session should be accessed when created")
	}

	attrs := map[string]any{"user": "ana"}
	s := NewSession(SessOptions{Attrs: attrs, Timeout: time.Hour})
	attrs["user"] = "changed"
	if s.Attr("user") != "ana" || s.Timeout() != time.Hour {
		t.Errorf("options not applied or attrs not copied: %v %v", s.Attr("user"), s.Timeout())
	}
}

func TestAttrs(t *testing.T) {
	s := NewSession()
	s.SetAttr("a", 1)
	s.SetAttr("b", "x")
	s.SetAttr("b", nil)
	if s.Attr("a") != 1 || s.Attr("b") != nil {
		t.Errorf("a=%v b=%v", s.Attr("a"), s.Attr("b"))
	}
	m := s.Attrs()
	m["a"] = 2
	if s.Attr("a") != 1 || len(m) != 1 {
		t.Error("Attrs must return a copy with the set attributes")
	}
}

func TestAddGetRemove(t *testing.T) {
	st := newStore(Options{})
	defer st.close()

	s := NewSession()
	rec := httptest.NewRecorder()
	st.add(s, rec)
	c := cookie(t, rec)
	if c.Name != "sessid" || c.Value != s.ID() || c.Path != "/" || !c.HttpOnly || !c.Secure || c.MaxAge != 30*24*3600 {
		t.Errorf("cookie %+v", c)
	}

	if got := st.get(requestWith(rec)); got != s {
		t.Fatalf("get: %v, want the added session", got)
	}
	if st.get(httptest.NewRequest("GET", "/", nil)) != nil {
		t.Error("a request without cookie has no session")
	}

	rec2 := httptest.NewRecorder()
	st.remove(s, rec2)
	if c := cookie(t, rec2); c.Value != "" || c.MaxAge != -1 {
		t.Errorf("remove should clear the cookie: %+v", c)
	}
	if st.get(requestWith(rec)) != nil || st.len() != 0 {
		t.Error("removed session still stored")
	}
}

func TestCookieOptions(t *testing.T) {
	st := newStore(Options{CookieName: "id", AllowHTTP: true, CookieMaxAge: -1, CookiePath: "/app"})
	defer st.close()
	rec := httptest.NewRecorder()
	st.add(NewSession(), rec)
	c := cookie(t, rec)
	if c.Name != "id" || c.Secure || c.MaxAge != 0 || c.Path != "/app" {
		t.Errorf("cookie %+v", c)
	}
}

func TestExpiry(t *testing.T) {
	st := newStore(Options{CleanInterval: time.Hour})
	defer st.close()
	s := NewSession(SessOptions{Timeout: 20 * time.Millisecond})
	rec := httptest.NewRecorder()
	st.add(s, rec)

	time.Sleep(40 * time.Millisecond)
	if st.get(requestWith(rec)) != nil {
		t.Error("an expired session must not be returned")
	}
	if st.len() != 0 {
		t.Error("an expired session found by get should be removed")
	}
}

func TestGetKeepsSessionAlive(t *testing.T) {
	st := newStore(Options{CleanInterval: time.Hour})
	defer st.close()
	s := NewSession(SessOptions{Timeout: 60 * time.Millisecond})
	rec := httptest.NewRecorder()
	st.add(s, rec)
	for i := 0; i < 4; i++ {
		time.Sleep(25 * time.Millisecond)
		if st.get(requestWith(rec)) == nil {
			t.Fatalf("session expired at round %d despite use", i)
		}
	}
}

func TestCleaner(t *testing.T) {
	st := newStore(Options{CleanInterval: 10 * time.Millisecond})
	defer st.close()
	for i := 0; i < 3; i++ {
		st.add(NewSession(SessOptions{Timeout: time.Millisecond}), httptest.NewRecorder())
	}
	st.add(NewSession(SessOptions{Timeout: time.Hour}), httptest.NewRecorder())

	deadline := time.Now().Add(time.Second)
	for st.len() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := st.len(); n != 1 {
		t.Errorf("after cleaning: %d sessions, want 1", n)
	}
}

func TestMaxSessionsEvictsLeastRecentlyUsed(t *testing.T) {
	st := newStore(Options{MaxSessions: 2})
	defer st.close()
	a, b, c := NewSession(), NewSession(), NewSession()
	recA := httptest.NewRecorder()
	st.add(a, recA)
	st.add(b, httptest.NewRecorder())
	st.get(requestWith(recA)) // a is now more recent than b
	st.add(c, httptest.NewRecorder())

	if st.len() != 2 {
		t.Fatalf("%d sessions, want 2", st.len())
	}
	st.mu.Lock()
	_, hasA := st.byID[a.ID()]
	_, hasB := st.byID[b.ID()]
	st.mu.Unlock()
	if !hasA || hasB {
		t.Errorf("want b evicted and a kept: a=%v b=%v", hasA, hasB)
	}
}

func TestSetLimit(t *testing.T) {
	st := newStore(Options{})
	defer st.close()
	for i := 0; i < 5; i++ {
		st.add(NewSession(), httptest.NewRecorder())
	}
	st.setLimit(3)
	if st.len() != 5 {
		t.Error("a new limit must not drop stored sessions by itself")
	}
	st.add(NewSession(), httptest.NewRecorder())
	if st.len() != 3 {
		t.Errorf("after add: %d sessions, want 3", st.len())
	}
	st.setLimit(0)
	for i := 0; i < 5; i++ {
		st.add(NewSession(), httptest.NewRecorder())
	}
	if st.len() != 8 {
		t.Errorf("unlimited: %d sessions, want 8", st.len())
	}
}

func TestIndexRebuiltAfterShrinking(t *testing.T) {
	st := newStore(Options{CleanInterval: time.Hour})
	defer st.close()
	for i := 0; i < 2000; i++ {
		st.add(NewSession(SessOptions{Timeout: time.Millisecond}), httptest.NewRecorder())
	}
	keep := NewSession(SessOptions{Timeout: time.Hour})
	rec := httptest.NewRecorder()
	st.add(keep, rec)
	st.mu.Lock()
	before := fmt.Sprintf("%p", st.byID)
	st.mu.Unlock()

	st.removeExpired(time.Now().Add(time.Second))

	st.mu.Lock()
	rebuilt := fmt.Sprintf("%p", st.byID) != before
	st.mu.Unlock()
	if !rebuilt || st.len() != 1 || st.get(requestWith(rec)) != keep {
		t.Errorf("rebuilt=%v len=%d: want a rebuilt index holding the live session", rebuilt, st.len())
	}
}

func TestPackageLevelStore(t *testing.T) {
	Init(Options{AllowHTTP: true})
	defer Reinit()

	s := NewSession()
	rec := httptest.NewRecorder()
	Add(s, rec)
	if Get(requestWith(rec)) != s || Len() != 1 {
		t.Fatal("package-level Add/Get")
	}
	Init(Options{})
	if Len() != 0 || Get(requestWith(rec)) != nil {
		t.Error("Init must discard stored sessions")
	}
	Remove(s, httptest.NewRecorder())
	SetMaxSessions(1)
	Add(NewSession(), httptest.NewRecorder())
	Add(NewSession(), httptest.NewRecorder())
	if Len() != 1 {
		t.Errorf("SetMaxSessions(1): %d sessions", Len())
	}
}

// Swapping the store while requests use it must not race or panic.
func TestConcurrentUseAndInit(t *testing.T) {
	defer Reinit()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s := NewSession()
				rec := httptest.NewRecorder()
				Add(s, rec)
				if got := Get(requestWith(rec)); got != nil {
					got.SetAttr("n", i)
				}
				Len()
				Remove(s, httptest.NewRecorder())
			}
		}()
	}
	for i := 0; i < 20; i++ {
		Init(Options{AllowHTTP: true})
	}
	wg.Wait()
}
