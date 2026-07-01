package gserver

import (
	"context"
	"crypto/sha256"
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

// userCookieKey is the HMAC key authenticating the "userid" session cookie.
// It defaults to a fixed development key and can be overridden at startup with
// SetUserCookieKey (typically from an environment-supplied secret).
var userCookieKey = []byte("f8hk39o9mx0dmrn1pa39jfla39djm3f0")

// SetUserCookieKey sets the session cookie HMAC key. key may be any length; it
// is hashed to the 32 bytes securecookie requires. An empty key is ignored,
// leaving the default in place. Call once at startup before serving.
func SetUserCookieKey(key []byte) {
	if len(key) == 0 {
		return
	}
	h := sha256.Sum256(key)
	userCookieKey = h[:]
}
