package api

import (
	"encoding/json"
	"net/mail"
	"strings"
)

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// validEmail reports whether s is a syntactically valid email.
func validEmail(s string) bool {
	_, err := mail.ParseAddress(s)
	return err == nil && len(s) <= 254
}

// reservedSubdomains cannot be used as instance subdomains (spec §26).
var reservedSubdomains = map[string]bool{
	"www": true, "api": true, "admin": true, "render": true, "console": true,
	"app": true, "mail": true, "preview": true, "static": true, "assets": true,
}

// validSubdomain checks DNS-label rules and reserved words (spec §26).
func validSubdomain(s string) bool {
	s = strings.ToLower(s)
	if len(s) < 2 || len(s) > 63 || reservedSubdomains[s] {
		return false
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			return false
		}
		if r == '-' && (i == 0 || i == len(s)-1) {
			return false
		}
	}
	return true
}

// strongPassword enforces a minimal password policy (spec §14/§25).
func strongPassword(p string) bool {
	if len(p) < 10 || len(p) > 256 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	return hasLetter && hasDigit
}
