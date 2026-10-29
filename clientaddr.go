package gserver

import (
	"net"
	"net/http"
	"strings"
)

// clientAddr returns the address of the client that made the request. Behind
// a reverse proxy on the same machine (Apache, nginx) RemoteAddr is the
// proxy's, so the address appended by the proxy to X-Forwarded-For is used
// instead. The header is only trusted when the request comes from loopback;
// otherwise any client could set it.
func clientAddr(r *http.Request) string {

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return r.RemoteAddr
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return r.RemoteAddr
	}

	// The proxy appends the address it saw; earlier entries come from the
	// client and cannot be trusted.
	if i := strings.LastIndexByte(xff, ','); i >= 0 {
		xff = xff[i+1:]
	}
	return strings.TrimSpace(xff)
}
