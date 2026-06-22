// Package id generates non-enumerable public IDs and high-entropy secrets.
// Spec §"ID 与 secret 编码规范": all public locators are random, non-sequential;
// secrets carry >=128 bit entropy from a CSPRNG, base62-encoded, with a
// purpose prefix. ID prefixes are for routing/readability only and carry no
// secrecy.
package id

import (
	"crypto/rand"
	"math/big"
)

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Prefixes for public locator IDs (non-secret).
const (
	PrefixTenant     = "tnt_"
	PrefixUser       = "usr_"
	PrefixMembership = "mbr_"
	PrefixInstance   = "inst_"
	PrefixFileRef    = "fref_"
	PrefixFile       = "file_"
	PrefixLink       = "plink_"
	PrefixDomain     = "dom_"
	PrefixAudit      = "evt_"
	PrefixReport     = "rpt_"
	PrefixSession    = "sess_"
	PrefixRequest    = "req_"
)

// Prefixes for secrets (high-entropy, hashed at rest).
const (
	SecretPreviewLink = "aps_" // preview link secret
	SecretFile        = "afs_" // cloud file direct-link secret
	SecretAgentToken  = "apage_agt_"
	SecretInstanceKey = "apage_key_"
)

// New returns a non-enumerable locator ID with the given prefix.
// 22 base62 chars ≈ 130 bits, far beyond enumeration range.
func New(prefix string) string {
	return prefix + randString(22)
}

// NewSecret returns a high-entropy secret with the given prefix.
// 30 base62 chars ≈ 178 bits, exceeding the 128-bit minimum (spec §15).
func NewSecret(prefix string) string {
	return prefix + randString(30)
}

// randString returns a cryptographically random base62 string of length n.
func randString(n int) string {
	out := make([]byte, n)
	max := big.NewInt(int64(len(base62)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// rand.Reader failure is fatal for security-sensitive code.
			panic("apage/id: CSPRNG unavailable: " + err.Error())
		}
		out[i] = base62[idx.Int64()]
	}
	return string(out)
}
