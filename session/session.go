// Copyright 2026, Rolf Veen.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package session keeps server-side HTTP sessions in memory, identified by a
// random ID carried in a cookie.
//
// A package-level store is ready to use: Get, Add, Remove, Len. Init replaces
// it with one configured by Options.
package session

import (
	"container/list"
	"crypto/rand"
	"encoding/base64"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Session is one client's session. Its methods are safe for concurrent use.
type Session struct {
	id      string
	created time.Time
	timeout time.Duration

	mu       sync.Mutex
	accessed time.Time
	attrs    map[string]any
}

// SessOptions configures a new Session.
type SessOptions struct {
	// Attributes the session starts with. The map is copied.
	Attrs map[string]any
	// Inactivity after which the session expires. Default: 30 minutes.
	Timeout time.Duration
}

const defaultTimeout = 30 * time.Minute

// NewSession returns a session with a fresh random ID. At most one SessOptions
// is used.
func NewSession(o ...SessOptions) *Session {
	var opt SessOptions
	if len(o) > 0 {
		opt = o[0]
	}
	if opt.Timeout <= 0 {
		opt.Timeout = defaultTimeout
	}
	now := time.Now()
	s := &Session{
		id:       newID(),
		created:  now,
		accessed: now,
		timeout:  opt.Timeout,
		attrs:    make(map[string]any, len(opt.Attrs)),
	}
	maps.Copy(s.attrs, opt.Attrs)
	return s
}

// newID returns 144 random bits, URL-safe base64 encoded (24 characters).
func newID() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("session: no randomness: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// ID returns the session's identifier, the value of its cookie.
func (s *Session) ID() string { return s.id }

// Created returns when the session was created.
func (s *Session) Created() time.Time { return s.created }

// Timeout returns the inactivity after which the session expires.
func (s *Session) Timeout() time.Duration { return s.timeout }

// Accessed returns when the session was last returned by Get (or created).
func (s *Session) Accessed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accessed
}

// Attr returns the named attribute, or nil.
func (s *Session) Attr(name string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attrs[name]
}

// SetAttr sets the named attribute; a nil value deletes it.
func (s *Session) SetAttr(name string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == nil {
		delete(s.attrs, name)
	} else {
		s.attrs[name] = value
	}
}

// Attrs returns a copy of all attributes.
func (s *Session) Attrs() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[string]any, len(s.attrs))
	maps.Copy(m, s.attrs)
	return m
}

// expired reports whether the session has been idle longer than its timeout.
func (s *Session) expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.accessed) > s.timeout
}

func (s *Session) touch(now time.Time) {
	s.mu.Lock()
	s.accessed = now
	s.mu.Unlock()
}

// Options configures a store.
type Options struct {
	// Name of the cookie holding the session ID. Default: "sessid".
	CookieName string
	// Send the cookie over plain HTTP too. Default: false (Secure cookie).
	AllowHTTP bool
	// Lifetime of the cookie. Default: 30 days. A negative value makes it a
	// browser-session cookie, dropped when the browser closes.
	CookieMaxAge time.Duration
	// Cookie path. Default: "/".
	CookiePath string
	// How often expired sessions are removed. Default: 10 seconds.
	CleanInterval time.Duration
	// Maximum number of stored sessions; 0 means no limit. When the store is
	// full, Add drops the least recently used sessions, so Add never fails.
	MaxSessions int
}

func (o Options) withDefaults() Options {
	if o.CookieName == "" {
		o.CookieName = "sessid"
	}
	if o.CookieMaxAge == 0 {
		o.CookieMaxAge = 30 * 24 * time.Hour
	}
	if o.CookiePath == "" {
		o.CookiePath = "/"
	}
	if o.CleanInterval <= 0 {
		o.CleanInterval = 10 * time.Second
	}
	return o
}

// store holds sessions by ID, with a recency list for eviction: the front is
// the most recently used session.
type store struct {
	opts Options

	mu    sync.Mutex
	byID  map[string]*list.Element // value: *Session
	lru   *list.List
	limit int
	peak  int // largest size since byID was last rebuilt

	stop      chan struct{}
	closeOnce sync.Once
}

func newStore(o Options) *store {
	st := &store{
		opts:  o.withDefaults(),
		byID:  map[string]*list.Element{},
		lru:   list.New(),
		limit: o.MaxSessions,
		stop:  make(chan struct{}),
	}
	go st.clean()
	return st
}

// get returns the live session named by r's cookie, or nil. An expired
// session is removed rather than returned.
func (st *store) get(r *http.Request) *Session {
	c, err := r.Cookie(st.opts.CookieName)
	if err != nil {
		return nil
	}
	now := time.Now()
	st.mu.Lock()
	defer st.mu.Unlock()
	e := st.byID[c.Value]
	if e == nil {
		return nil
	}
	s := e.Value.(*Session)
	if s.expired(now) {
		st.removeLocked(e)
		return nil
	}
	s.touch(now)
	st.lru.MoveToFront(e)
	return s
}

// add stores s, making room if the store is full, and sets its cookie on w.
func (st *store) add(s *Session, w http.ResponseWriter) {
	st.mu.Lock()
	if e := st.byID[s.id]; e != nil {
		e.Value = s
		st.lru.MoveToFront(e)
	} else {
		for st.limit > 0 && st.lru.Len() >= st.limit {
			st.removeLocked(st.lru.Back())
		}
		st.byID[s.id] = st.lru.PushFront(s)
		st.peak = max(st.peak, st.lru.Len())
	}
	st.mu.Unlock()

	maxAge := int(st.opts.CookieMaxAge / time.Second)
	if st.opts.CookieMaxAge < 0 {
		maxAge = 0 // no Max-Age: a browser-session cookie
	}
	http.SetCookie(w, &http.Cookie{
		Name:     st.opts.CookieName,
		Value:    s.id,
		Path:     st.opts.CookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   !st.opts.AllowHTTP,
	})
}

// remove forgets s and tells the client to drop its cookie.
func (st *store) remove(s *Session, w http.ResponseWriter) {
	st.mu.Lock()
	if e := st.byID[s.id]; e != nil {
		st.removeLocked(e)
	}
	st.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     st.opts.CookieName,
		Value:    "",
		Path:     st.opts.CookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !st.opts.AllowHTTP,
	})
}

func (st *store) removeLocked(e *list.Element) {
	delete(st.byID, e.Value.(*Session).id)
	st.lru.Remove(e)
}

func (st *store) len() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.lru.Len()
}

func (st *store) setLimit(n int) {
	st.mu.Lock()
	st.limit = n
	st.mu.Unlock()
}

// clean removes expired sessions every CleanInterval until close.
func (st *store) clean() {
	t := time.NewTicker(st.opts.CleanInterval)
	defer t.Stop()
	for {
		select {
		case <-st.stop:
			return
		case now := <-t.C:
			st.removeExpired(now)
		}
	}
}

// removeExpired drops expired sessions. Since a Go map never gives back the
// memory of deleted entries, the index is rebuilt once it has shrunk to a
// quarter of its peak size, so a burst of sessions does not pin memory.
func (st *store) removeExpired(now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for e := st.lru.Back(); e != nil; {
		prev := e.Prev()
		if e.Value.(*Session).expired(now) {
			st.removeLocked(e)
		}
		e = prev
	}
	if n := st.lru.Len(); st.peak >= 1024 && n <= st.peak/4 {
		byID := make(map[string]*list.Element, n)
		maps.Copy(byID, st.byID)
		st.byID = byID
		st.peak = n
	}
}

func (st *store) close() {
	st.closeOnce.Do(func() { close(st.stop) })
}

// The package-level store.
var current atomic.Pointer[store]

func init() { Reinit() }

// Init replaces the package-level store with an empty one configured by o,
// discarding all sessions. Meant for startup and tests, not for reconfiguring
// a running server.
func Init(o Options) {
	if old := current.Swap(newStore(o)); old != nil {
		old.close()
	}
}

// Reinit is Init with the package defaults: cookies allowed over plain HTTP
// (TLS is a proxy's job) and a clean-up every 30 minutes.
func Reinit() { Init(Options{AllowHTTP: true, CleanInterval: 30 * time.Minute}) }

// Get returns the live session for r, or nil.
func Get(r *http.Request) *Session { return current.Load().get(r) }

// Add stores s and sets its cookie on w.
func Add(s *Session, w http.ResponseWriter) { current.Load().add(s, w) }

// Remove deletes s and clears its cookie on w.
func Remove(s *Session, w http.ResponseWriter) { current.Load().remove(s, w) }

// Len returns the number of stored sessions.
func Len() int { return current.Load().len() }

// SetMaxSessions changes the session limit; 0 means none. It takes effect on
// the next Add and does not drop sessions already stored.
func SetMaxSessions(n int) { current.Load().setLimit(n) }

// Close stops the package-level store's clean-up.
func Close() { current.Load().close() }
