package daemon

import (
	"fmt"
	"net"
	"net/http"
)

// getRoutes are endpoints that accept GET. Everything else is POST-only.
// GET here is what browsers can issue cross-origin without preflight, so
// it is reserved for read-only endpoints; every mutating command is POST.
var getRoutes = map[string]bool{
	"/ping":          true,
	"/events/stream": true,
	"/debug/pid":     true,
	"/tab/list":      true,
	"/record/list":   true,
	"/session/list":  true,
	"/downloads":     true,
}

// secureMiddleware enforces two invariants for a localhost daemon:
//
//  1. The Host header must be the daemon's own loopback address. Without
//     this, DNS rebinding lets a remote page become same-origin with the
//     daemon and drive any endpoint (js execution, file writes, shutdown).
//  2. Mutating endpoints only accept POST. Without this, a cross-site img
//     tag or link prefetch can trigger GET /shutdown or other state changes
//     from any web page the operator visits.
func (s *Server) secureMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		hostNoPort := host
		if h, _, err := net.SplitHostPort(host); err == nil {
			hostNoPort = h
		}
		if !isLoopbackHost(hostNoPort) {
			http.Error(w, fmt.Sprintf("host %q not allowed; the daemon serves localhost only", host), http.StatusForbidden)
			return
		}

		if getRoutes[r.URL.Path] {
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		} else if r.Method != http.MethodPost {
			http.Error(w, fmt.Sprintf("method %s not allowed; use POST", r.Method), http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
