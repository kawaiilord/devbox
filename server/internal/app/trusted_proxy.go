package app

import (
	"net"
	"net/http"
	"strings"
)

// Only explicitly configured proxy networks may supply the client address.
// The deployment proxy overwrites X-Real-IP rather than forwarding a client value.
func (s *Server) withTrustedProxy(next http.Handler) http.Handler {
	networks := []*net.IPNet{}
	for _, raw := range s.options.TrustedProxyCIDRs {
		_, network, e := net.ParseCIDR(strings.TrimSpace(raw))
		if e == nil {
			networks = append(networks, network)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := net.ParseIP(sourceIdentity(r))
		real := net.ParseIP(r.Header.Get("X-Real-IP"))
		if peer != nil && real != nil {
			for _, network := range networks {
				if network.Contains(peer) {
					r = r.Clone(r.Context())
					r.RemoteAddr = net.JoinHostPort(real.String(), "0")
					break
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
