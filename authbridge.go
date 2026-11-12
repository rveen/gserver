package gserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"log"
	"os"
	"sync"
)

// authbridge.go lets an upstream Authenticate middleware (see
// github.com/trukeio/gserver/auth) inject an already-resolved identity into a
// request so that the existing per-request SessionContext machinery in
// getSession() picks it up. This is the bridge that keeps the template and
// data-layer authorization (which read the "user"/"userACL" scalars) working
// unchanged for the stateless channels (bearer token, trusted header) that do
// not use the session cookie. Session-establishing channels (local, oidc)
// continue to use the userid cookie and need none of this.

type authUserKeyType struct{}

var authUserKey authUserKeyType

// InjectedUser is an externally-resolved identity carried on the request
// context. UID maps to the "user" scalar; ACL, when non-empty, maps to the
// "userACL" scalar (a space-separated label set), bypassing the GetACL lookup.
type InjectedUser struct {
	UID string
	ACL string
}

// WithUser returns a copy of ctx carrying u, to be read by getSession().
func WithUser(ctx context.Context, u *InjectedUser) context.Context {
	return context.WithValue(ctx, authUserKey, u)
}

// userFromContext returns the injected identity, or nil.
func userFromContext(ctx context.Context) *InjectedUser {
	u, _ := ctx.Value(authUserKey).(*InjectedUser)
	return u
}

// CookieKeyEnv names the environment variable that sets the cookie key.
const CookieKeyEnv = "GSERVER_COOKIE_KEY"

// userCookieKey is the HMAC key authenticating the "userid" and "redirect"
// cookies. It comes from SetUserCookieKey, else from the GSERVER_COOKIE_KEY
// environment variable, else it is random for this process: then nobody can
// forge a cookie, but logins end when the server restarts.
var userCookieKey, userCookieKeySet = initialCookieKey()

var warnRandomKey sync.Once

func initialCookieKey() ([]byte, bool) {
	if k := os.Getenv(CookieKeyEnv); k != "" {
		h := sha256.Sum256([]byte(k))
		return h[:], true
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("gserver: cannot generate a cookie key: " + err.Error())
	}
	return b, false
}

// cookieKey returns the cookie key, warning once if it is the random one.
func cookieKey() []byte {
	if !userCookieKeySet {
		warnRandomKey.Do(func() {
			log.Printf("gserver: no cookie key configured; using a random one, so logins end when the server restarts. Set %s to keep them.", CookieKeyEnv)
		})
	}
	return userCookieKey
}

// SetUserCookieKey sets the cookie HMAC key, overriding GSERVER_COOKIE_KEY.
// key may be any length; it is hashed to the 32 bytes securecookie requires.
// An empty key is ignored. Call once at startup before serving.
func SetUserCookieKey(key []byte) {
	if len(key) == 0 {
		return
	}
	h := sha256.Sum256(key)
	userCookieKey = h[:]
	userCookieKeySet = true
}
