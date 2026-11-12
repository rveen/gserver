package gserver

import (
	"crypto/sha256"
	"net/http/httptest"
	"testing"

	"github.com/chmike/securecookie"
)

// The key that was hard-coded in this package's source until it became
// random by default. A cookie signed with it must not be accepted.
var formerPublicKey = []byte("f8hk39o9mx0dmrn1pa39jfla39djm3f0")

// keepCookieKey restores the package's cookie key after a test changes it.
func keepCookieKey(t *testing.T) {
	key, set := userCookieKey, userCookieKeySet
	t.Cleanup(func() { userCookieKey, userCookieKeySet = key, set })
}

func TestForgedUserCookieRejected(t *testing.T) {
	keepCookieKey(t)
	userCookieKey, userCookieKeySet = initialCookieKey() // no env var: random

	forger := securecookie.MustNew("userid", formerPublicKey, securecookie.Params{Path: "/"})
	rec := httptest.NewRecorder()
	if err := forger.SetValue(rec, []byte("admin")); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	if got := UserCookieValue(r); got != "" {
		t.Errorf("cookie signed with the former public key accepted as user %q", got)
	}
}

func TestUserCookieRoundTrip(t *testing.T) {
	keepCookieKey(t)
	userCookieKey, userCookieKeySet = initialCookieKey()

	rec := httptest.NewRecorder()
	if err := UserCookie().SetValue(rec, []byte("ana")); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	if got := UserCookieValue(r); got != "ana" {
		t.Errorf("got %q, want %q", got, "ana")
	}
}

func TestCookieKeySources(t *testing.T) {
	keepCookieKey(t)

	t.Setenv(CookieKeyEnv, "")
	a, setA := initialCookieKey()
	b, _ := initialCookieKey()
	if setA || len(a) != 32 || string(a) == string(b) {
		t.Errorf("without %s: want a random 32-byte key per call, unset", CookieKeyEnv)
	}

	t.Setenv(CookieKeyEnv, "site secret")
	k, set := initialCookieKey()
	want := sha256.Sum256([]byte("site secret"))
	if !set || string(k) != string(want[:]) {
		t.Errorf("%s not applied", CookieKeyEnv)
	}

	userCookieKey, userCookieKeySet = a, false
	SetUserCookieKey([]byte("from code"))
	want = sha256.Sum256([]byte("from code"))
	if !userCookieKeySet || string(userCookieKey) != string(want[:]) {
		t.Error("SetUserCookieKey not applied")
	}
}
