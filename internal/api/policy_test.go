package api

import (
	"testing"
	"time"

	"github.com/apage/apage/internal/store"
)

func TestEffectiveExpiry(t *testing.T) {
	now := time.Now()
	earlier := now.Add(time.Hour)
	later := now.Add(2 * time.Hour)

	if got := effectiveExpiry(&later, &earlier); !got.Equal(earlier) {
		t.Fatalf("most-restrictive should win: got %v want %v", got, earlier)
	}
	if got := effectiveExpiry(&earlier, nil); !got.Equal(earlier) {
		t.Fatalf("nil backing should yield link expiry")
	}
	if got := effectiveExpiry(nil, &later); !got.Equal(later) {
		t.Fatalf("nil link should yield backing expiry")
	}
	if got := effectiveExpiry(nil, nil); got != nil {
		t.Fatalf("both nil should be nil")
	}
}

func TestMaxViewsCapSingleUse(t *testing.T) {
	if cap := maxViewsCap(store.AccessPolicy{SingleUse: true, MaxViews: 100}); cap != 1 {
		t.Fatalf("single_use must cap at 1, got %d", cap)
	}
	if cap := maxViewsCap(store.AccessPolicy{MaxViews: 5}); cap != 5 {
		t.Fatalf("expected 5, got %d", cap)
	}
}

func TestIPAllowed(t *testing.T) {
	pol := store.AccessPolicy{IPAllowlist: []string{"203.0.113.0/24"}}
	if !ipAllowed(pol, "203.0.113.5") {
		t.Fatal("expected in-range IP allowed")
	}
	if ipAllowed(pol, "198.51.100.1") {
		t.Fatal("expected out-of-range IP denied")
	}
	if !ipAllowed(store.AccessPolicy{}, "1.2.3.4") {
		t.Fatal("empty allowlist should allow all")
	}
}

func TestPolicyPasswordHashRoundTripAndRedaction(t *testing.T) {
	raw := []byte(`{"type":"password","allowDownload":true,"password":{"enabled":true,"hash":"argon2id$secret","attemptLimit":5}}`)
	p := parsePolicy(raw)
	if p.Password == nil || p.Password.Hash != "argon2id$secret" {
		t.Fatal("parsePolicy must load the password hash for runtime verification")
	}
	red := redactPolicy(raw)
	if string(red) == "" || contains(string(red), "argon2id$secret") {
		t.Fatalf("redactPolicy must strip the hash, got %s", red)
	}
	// Redacted policy still parses and still reports password required.
	if !passwordRequired(parsePolicy(red)) {
		t.Fatal("redacted policy should still indicate password is required")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestPreviewCategory(t *testing.T) {
	cases := map[string]string{
		"application/pdf":  "pdf",
		"image/png":        "image",
		"image/svg+xml":    "svg",
		"text/html":        "html",
		"text/plain":       "text",
		"application/json": "text",
		"application/zip":  "binary",
	}
	for mime, want := range cases {
		if got := previewCategory(mime); got != want {
			t.Errorf("previewCategory(%q)=%q want %q", mime, got, want)
		}
	}
}
