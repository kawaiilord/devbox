package app

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type proxyProvenanceKey struct{}
type proxyProvenance struct{ Peer, Source string }

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
		provenance := proxyProvenance{Peer: sourceIdentity(r), Source: "direct"}
		peer := net.ParseIP(sourceIdentity(r))
		real := net.ParseIP(r.Header.Get("X-Real-IP"))
		if peer != nil {
			for _, network := range networks {
				if network.Contains(peer) {
					provenance.Source = "unverified_proxy"
					if real != nil && len(r.Header.Values("X-Real-IP")) == 1 {
						r = r.Clone(r.Context())
						r.RemoteAddr = net.JoinHostPort(real.String(), "0")
						provenance.Source = "trusted_proxy"
					}
					break
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), proxyProvenanceKey{}, provenance)))
	})
}
