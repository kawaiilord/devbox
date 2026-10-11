package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyRequiresExplicitPeerNetwork(t *testing.T) {
	s := &Server{options: Options{TrustedProxyCIDRs: []string{"172.19.0.0/16"}}}
	for _, test := range []struct{ peer, header, want string }{{"172.19.0.5:123", "203.0.113.2", "203.0.113.2"}, {"203.0.113.10:123", "203.0.113.2", "203.0.113.10"}, {"172.19.0.5:123", "invalid", "172.19.0.5"}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("X-Real-IP", test.header)
		h := s.withTrustedProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := sourceIdentity(r); got != test.want {
				t.Errorf("source=%s want=%s", got, test.want)
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
}
