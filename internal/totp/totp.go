// Package totp implements RFC 6238 time-based one-time passwords (SHA1, 6
// digits, 30s period) for admin MFA — self-contained, no external dependency.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const period = 30

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random base32 TOTP secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return enc.EncodeToString(b), nil
}

// Code computes the 6-digit code for a secret at time t (RFC 6238).
func Code(secret string, t time.Time) (string, error) {
	key, err := enc.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(t.Unix())/period)
	h := hmac.New(sha1.New, key)
	h.Write(buf[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	val := (uint32(sum[off]&0x7f) << 24) | (uint32(sum[off+1]) << 16) | (uint32(sum[off+2]) << 8) | uint32(sum[off+3])
	return fmt.Sprintf("%06d", val%1000000), nil
}

// Verify checks a code against the secret for the current and adjacent windows
// (±1 step) to tolerate clock skew. Constant-time comparison.
func Verify(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	for _, skew := range []time.Duration{0, -period * time.Second, period * time.Second} {
		if c, err := Code(secret, t.Add(skew)); err == nil && subtle.ConstantTimeCompare([]byte(c), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OtpauthURI builds the otpauth:// URI an authenticator app scans to enroll.
func OtpauthURI(secret, account, issuer string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}
