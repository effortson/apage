package httpx

import (
	"net/http"
	"testing"
)

// TestClientIP verifies the real client IP is taken the configured number of
// hops from the right of X-Forwarded-For, so a client-spoofed leftmost value is
// ignored (security review #2).
func TestClientIP(t *testing.T) {
	cases := []struct {
		name        string
		remote      string
		xff         string
		trustedHops int
		want        string
	}{
		{"single edge override", "10.0.0.1:5", "1.2.3.4", 1, "1.2.3.4"},
		{"spoofed leftmost ignored", "10.0.0.1:5", "9.9.9.9, 1.2.3.4", 1, "1.2.3.4"},
		{"two trusted hops", "10.0.0.1:5", "9.9.9.9, 1.2.3.4, 7.7.7.7", 2, "1.2.3.4"},
		{"no xff falls back to peer", "5.6.7.8:1234", "", 1, "5.6.7.8"},
		{"xff ignored when untrusted", "5.6.7.8:1234", "9.9.9.9", 0, "5.6.7.8"},
		{"too few hops falls back to peer", "5.6.7.8:1234", "1.2.3.4", 2, "5.6.7.8"},
		{"ipv6 peer brackets stripped", "[2001:db8::1]:443", "", 1, "2001:db8::1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: c.remote, Header: http.Header{}}
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(r, c.trustedHops); got != c.want {
				t.Fatalf("clientIP=%q want %q", got, c.want)
			}
		})
	}
}
