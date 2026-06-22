package gateway

import (
	"net/http"
	"testing"

	"github.com/apage/apage/internal/config"
)

// TestInternalAuthorized verifies the internal stream endpoint enforces the
// API<->gateway shared secret when configured, and stays open in dev when unset
// (security review #3).
func TestInternalAuthorized(t *testing.T) {
	withSecret := &Server{cfg: &config.Config{GatewayInternalSecret: "s3cr3t"}}
	noSecret := &Server{cfg: &config.Config{GatewayInternalSecret: ""}}

	req := func(hdr string) *http.Request {
		r := &http.Request{Header: http.Header{}}
		if hdr != "" {
			r.Header.Set(internalAuthHeader, hdr)
		}
		return r
	}

	if !withSecret.internalAuthorized(req("s3cr3t")) {
		t.Error("matching secret must be authorized")
	}
	if withSecret.internalAuthorized(req("wrong")) {
		t.Error("wrong secret must be rejected")
	}
	if withSecret.internalAuthorized(req("")) {
		t.Error("missing secret must be rejected when one is configured")
	}
	if !noSecret.internalAuthorized(req("")) {
		t.Error("no configured secret => open (dev/single-box)")
	}
}
