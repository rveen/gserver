package gserver

import (
	"net/http/httptest"
	"testing"
)

func TestClientAddr(t *testing.T) {
	tests := []struct {
		remote, xff, want string
	}{
		{"203.0.113.5:4000", "", "203.0.113.5:4000"},
		{"203.0.113.5:4000", "198.51.100.1", "203.0.113.5:4000"}, // not from a local proxy
		{"127.0.0.1:4000", "", "127.0.0.1:4000"},
		{"127.0.0.1:4000", "198.51.100.1", "198.51.100.1"},
		{"[::1]:4000", "198.51.100.1", "198.51.100.1"},
		{"127.0.0.1:4000", "10.0.0.1, 198.51.100.1", "198.51.100.1"}, // first entry set by the client
	}
	for _, tc := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		if tc.xff != "" {
			r.Header.Set("X-Forwarded-For", tc.xff)
		}
		if got := clientAddr(r); got != tc.want {
			t.Errorf("remote %q xff %q: got %q, want %q", tc.remote, tc.xff, got, tc.want)
		}
	}
}
