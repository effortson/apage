package api

import "testing"

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"Tenant.Preview.Example.com.": "tenant.preview.example.com",
		"edge.example.com":            "edge.example.com",
		"  EDGE.EXAMPLE.COM.  ":       "edge.example.com",
	}
	for in, want := range cases {
		if got := normalizeHost(in); got != want {
			t.Errorf("normalizeHost(%q)=%q want %q", in, got, want)
		}
	}
	// Resolver canonical form (trailing dot, mixed case) must equal our target.
	if normalizeHost("ten_abc.preview.example.com.") != normalizeHost("ten_abc.preview.example.com") {
		t.Error("trailing-dot/case differences must compare equal")
	}
}
