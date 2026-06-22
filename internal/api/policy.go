package api

import (
	"encoding/json"
	"net"
	"time"

	"github.com/apage/apage/internal/store"
)

// effectiveExpiry implements three-layer expiry (spec §11): the valid window is
// the earliest of the link, the backing file/file_ref, and now. Returns the
// earliest non-nil expiry.
func effectiveExpiry(link *time.Time, backing *time.Time) *time.Time {
	switch {
	case link == nil:
		return backing
	case backing == nil:
		return link
	case backing.Before(*link):
		return backing
	default:
		return link
	}
}

// storedPolicy mirrors the persisted access policy including the password hash,
// which store.AccessPolicy intentionally hides from clients (json:"-").
type storedPolicy struct {
	Type          string   `json:"type"`
	AllowDownload bool     `json:"allowDownload"`
	IPAllowlist   []string `json:"ipAllowlist"`
	MaxViews      int      `json:"maxViews"`
	SingleUse     bool     `json:"singleUse"`
	Password      *struct {
		Enabled      bool   `json:"enabled"`
		Hash         string `json:"hash"`
		AttemptLimit int    `json:"attemptLimit"`
	} `json:"password"`
	Account *struct {
		Required         bool     `json:"required"`
		AllowedTenantIDs []string `json:"allowedTenantIds"`
		AllowedUserIDs   []string `json:"allowedUserIds"`
	} `json:"account"`
}

// parsePolicy decodes the stored access policy JSON, including the password hash
// for runtime verification (spec §14). Never serialize the result to clients.
func parsePolicy(raw json.RawMessage) store.AccessPolicy {
	var sp storedPolicy
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &sp)
	}
	p := store.AccessPolicy{
		Type: sp.Type, AllowDownload: sp.AllowDownload, IPAllowlist: sp.IPAllowlist,
		MaxViews: sp.MaxViews, SingleUse: sp.SingleUse,
	}
	if sp.Password != nil {
		p.Password = &struct {
			Enabled      bool   `json:"enabled"`
			Hash         string `json:"-"`
			AttemptLimit int    `json:"attemptLimit"`
		}{Enabled: sp.Password.Enabled, Hash: sp.Password.Hash, AttemptLimit: sp.Password.AttemptLimit}
	}
	if sp.Account != nil {
		p.Account = &struct {
			Required         bool     `json:"required"`
			AllowedTenantIDs []string `json:"allowedTenantIds"`
			AllowedUserIDs   []string `json:"allowedUserIds"`
		}{Required: sp.Account.Required, AllowedTenantIDs: sp.Account.AllowedTenantIDs, AllowedUserIDs: sp.Account.AllowedUserIDs}
	}
	if p.Type == "" {
		p.Type = "public_token"
	}
	return p
}

// redactPolicy strips the password hash from a stored policy before returning it
// to clients (spec §14 / UI §4.3: hash must never be exposed).
func redactPolicy(raw json.RawMessage) json.RawMessage {
	var sp storedPolicy
	if len(raw) == 0 || json.Unmarshal(raw, &sp) != nil {
		return raw
	}
	if sp.Password != nil {
		sp.Password.Hash = ""
	}
	b, err := json.Marshal(sp)
	if err != nil {
		return raw
	}
	return b
}

// ipAllowed checks the client IP against the policy CIDR allowlist (spec §14).
// Empty allowlist means no IP restriction.
func ipAllowed(p store.AccessPolicy, clientIP string) bool {
	if len(p.IPAllowlist) == 0 {
		return true
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, c := range p.IPAllowlist {
		if _, network, err := net.ParseCIDR(c); err == nil {
			if network.Contains(ip) {
				return true
			}
		} else if c == clientIP {
			return true
		}
	}
	return false
}

// maxViewsCap returns the effective maxViews cap: single_use means 1 (spec §14).
func maxViewsCap(p store.AccessPolicy) int {
	if p.SingleUse {
		return 1
	}
	return p.MaxViews
}

// passwordRequired reports whether the policy gates on a password (spec §14).
func passwordRequired(p store.AccessPolicy) bool {
	return p.Type == "password" || (p.Password != nil && p.Password.Enabled)
}

// accountRequired reports whether the policy requires a logged-in account (spec §14).
func accountRequired(p store.AccessPolicy) bool {
	return p.Type == "account" || (p.Account != nil && p.Account.Required)
}
