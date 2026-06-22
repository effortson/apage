package api

import (
	"strings"
	"testing"
)

// TestContentDispositionResistsHostileName verifies that an attacker-controlled
// display name cannot break out of the quoted filename or inject control bytes
// into the Content-Disposition header.
func TestContentDispositionResistsHostileName(t *testing.T) {
	cases := []struct {
		name string
		bad  []string // substrings that must NOT appear in the ASCII filename token
	}{
		{`evil".html`, []string{`"`}},
		{"back\\slash", []string{`\`}},
		{"line\r\nbreak", []string{"\r", "\n"}},
		{"ctrl\x00byte", []string{"\x00"}},
	}
	for _, c := range cases {
		got := contentDisposition("inline", c.name)
		// The quoted segment is between filename=" and "; filename*=
		start := strings.Index(got, `filename="`) + len(`filename="`)
		end := strings.Index(got, `"; filename*=`)
		if start < 0 || end < 0 || end < start {
			t.Fatalf("unexpected header shape: %q", got)
		}
		quoted := got[start:end]
		for _, b := range c.bad {
			if strings.Contains(quoted, b) {
				t.Errorf("name %q: quoted filename %q still contains %q", c.name, quoted, b)
			}
		}
	}
}

// TestRFC5987EscapeEncodesSpecials ensures the filename* parameter is percent
// encoded so spaces/quotes/UTF-8 cannot alter header parsing.
func TestRFC5987EscapeEncodesSpecials(t *testing.T) {
	got := rfc5987Escape(`a b"c`)
	if strings.ContainsAny(got, ` "`) {
		t.Errorf("rfc5987Escape left raw specials: %q", got)
	}
}
