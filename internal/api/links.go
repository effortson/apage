package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

type createLinkReq struct {
	Mode             string          `json:"mode"`
	FileID           string          `json:"fileId"`
	ExpiresInSeconds int64           `json:"expiresInSeconds"`
	DisplayName      string          `json:"displayName"`
	AccessPolicy     json.RawMessage `json:"accessPolicy"`
	Password         string          `json:"password"` // plaintext, hashed before storage
}

// handleCreateLink creates a cloud preview link (spec §8/§12). Links are created
// only by an agent authenticating with an instance api key — console sessions
// cannot create links (they manage/revoke via the console).
func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	if !sc.ViaKey || sc.Instance == nil {
		httpx.Forbidden(w, r, "preview links are created by the agent via its instance API key (MCP)")
		return
	}
	var req createLinkReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	if req.Mode == "" {
		req.Mode = "cloud"
	}
	if req.Mode != "cloud" {
		httpx.BadRequest(w, r, "mode must be cloud")
		return
	}
	if req.ExpiresInSeconds <= 0 {
		req.ExpiresInSeconds = 3600
	}
	// The agent's instance-key scope names the operating instance.
	in := sc.Instance
	if in != nil && in.FrozenAt != nil {
		httpx.Forbidden(w, r, "instance is frozen")
		return
	}
	instanceID := ""
	if in != nil {
		instanceID = in.InstanceID
	}

	// Resolve tenant trust + plan once: trust tightens the rate limit, plan caps
	// the link lifetime (spec §15.5 信任分级 / §11 three-layer expiry).
	trust, plan := "new", "lite"
	if t, err := s.db.TenantByID(r.Context(), sc.TenantID); err == nil {
		if t.Status == "suspended" {
			httpx.Forbidden(w, r, "tenant is suspended")
			return
		}
		if t.TrustLevel != "" {
			trust = t.TrustLevel
		}
		plan = t.Plan
	}
	if !s.limit(w, r, "linkcreate:"+sc.TenantID, linkCreateCap(trust), time.Minute) {
		return
	}

	s.idempotent(sc.idemScope(), "create-link", bodyHash(req), w, r, func() (int, any) {
		link := store.PreviewLink{
			LinkID: id.New(id.PrefixLink), TenantID: sc.TenantID, InstanceID: instanceID,
			Mode: req.Mode, DisplayName: req.DisplayName,
		}
		linkExpiry := time.Now().Add(time.Duration(req.ExpiresInSeconds) * time.Second)
		// Plan cap (third expiry layer): e.g. lite links may not exceed 24h (§20).
		if max := planMaxLinkTTL(plan); max > 0 {
			if cap := time.Now().Add(max); linkExpiry.After(cap) {
				linkExpiry = cap
			}
		}
		var backingExpiry *time.Time

		if req.FileID == "" {
			return badReq(r, "fileId required")
		}
		f, err := s.db.FileByID(r.Context(), sc.TenantID, req.FileID)
		if err != nil {
			return notFound(r)
		}
		if f.Status != "ready" {
			return conflict(r, "file not ready (status="+f.Status+"); only ready files can back a cloud link")
		}
		link.FileID = &f.FileID
		if link.DisplayName == "" {
			link.DisplayName = f.DisplayName
		}
		if link.InstanceID == "" {
			link.InstanceID = f.InstanceID // cloud link inherits the uploading instance's subdomain
		}
		backingExpiry = f.ExpiresAt

		// Clamp to backing's remaining lifetime (spec §11).
		eff := effectiveExpiry(&linkExpiry, backingExpiry)
		link.ExpiresAt = eff

		// Build/normalize access policy, hashing any password (spec §14).
		pol := parsePolicy(req.AccessPolicy)
		if req.Password != "" {
			ph, err := hash.Password(req.Password)
			if err != nil {
				return 500, internalBody(r)
			}
			pol.Type = "password"
			pol.Password = &struct {
				Enabled      bool   `json:"enabled"`
				Hash         string `json:"-"`
				AttemptLimit int    `json:"attemptLimit"`
			}{Enabled: true, Hash: ph, AttemptLimit: 5}
		}
		polJSON := marshalPolicyForStorage(pol, req.Password)
		link.AccessPolicy = polJSON

		secret := id.NewSecret(id.SecretPreviewLink)
		if err := s.db.CreateLink(r.Context(), link, hash.SecretHash(secret)); err != nil {
			s.log.Error("create link", "err", err)
			return 500, internalBody(r)
		}
		s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID, InstanceID: link.InstanceID,
			Event: audit.PreviewLinkCreated, ActorType: actorOf(sc), ActorID: actorID(sc),
			ResourceType: "preview_link", ResourceID: link.LinkID, IP: httpx.ClientIP(r.Context())})

		// Resolve the subdomain for the public URL (spec §8).
		sub := "preview"
		if in != nil {
			sub = in.Subdomain
		} else if link.InstanceID != "" {
			if li, err := s.db.InstanceByID(r.Context(), link.InstanceID); err == nil {
				sub = li.Subdomain
			}
		}
		url := "https://" + sub + "." + s.cfg.BaseDomain + "/p/" + link.LinkID + "/" + secret
		return http.StatusCreated, map[string]any{
			"linkId":    link.LinkID,
			"url":       url, // contains secret; shown once (spec §8)
			"expiresAt": link.ExpiresAt,
		}
	})
}

type updateLinkReq struct {
	FileID           *string         `json:"fileId"`
	DisplayName      *string         `json:"displayName"`
	ExpiresInSeconds *int64          `json:"expiresInSeconds"`
	AccessPolicy     json.RawMessage `json:"accessPolicy"`
	Password         *string         `json:"password"`
}

// handleUpdateLink modifies an existing link in place (modify_link): swap the
// backing cloud file and/or change display name, access policy/password, and
// expiry. The link id + secret are preserved so the public URL keeps working.
// Agent-only (instance API key); console sessions manage links read-only.
func (s *Server) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	if !sc.ViaKey || sc.Instance == nil {
		httpx.Forbidden(w, r, "preview links are managed by the agent via its instance API key (MCP)")
		return
	}
	linkID := chi.URLParam(r, "id")
	var req updateLinkReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}

	var upd store.LinkUpdate
	var backingExpiry *time.Time
	if req.FileID != nil {
		f, err := s.db.FileByID(r.Context(), sc.TenantID, *req.FileID)
		if err != nil {
			httpx.NotFound(w, r)
			return
		}
		if f.Status != "ready" {
			httpx.Conflict(w, r, "file not ready (status="+f.Status+"); only ready files can back a cloud link")
			return
		}
		upd.FileID = &f.FileID
		backingExpiry = f.ExpiresAt
	}
	if req.DisplayName != nil {
		upd.DisplayName = req.DisplayName
	}
	// Rebuild the stored access policy when a policy or password is supplied.
	if req.AccessPolicy != nil || req.Password != nil {
		pol := parsePolicy(req.AccessPolicy)
		pw := ""
		if req.Password != nil {
			pw = *req.Password
		}
		if pw != "" {
			ph, err := hash.Password(pw)
			if err != nil {
				httpx.Internal(w, r)
				return
			}
			pol.Type = "password"
			pol.Password = &struct {
				Enabled      bool   `json:"enabled"`
				Hash         string `json:"-"`
				AttemptLimit int    `json:"attemptLimit"`
			}{Enabled: true, Hash: ph, AttemptLimit: 5}
		}
		upd.AccessPolicy = marshalPolicyForStorage(pol, pw)
	}
	if req.ExpiresInSeconds != nil && *req.ExpiresInSeconds > 0 {
		plan := "lite"
		if t, err := s.db.TenantByID(r.Context(), sc.TenantID); err == nil {
			plan = t.Plan
		}
		exp := time.Now().Add(time.Duration(*req.ExpiresInSeconds) * time.Second)
		if max := planMaxLinkTTL(plan); max > 0 {
			if cap := time.Now().Add(max); exp.After(cap) {
				exp = cap
			}
		}
		upd.ExpiresAt = effectiveExpiry(&exp, backingExpiry)
	}

	if err := s.db.UpdateLink(r.Context(), sc.TenantID, linkID, upd); err != nil {
		if err == store.ErrNotFound {
			httpx.NotFound(w, r)
			return
		}
		httpx.Internal(w, r)
		return
	}
	// Invalidate cache so the swapped content/policy takes effect <=5s (spec §19.7).
	_ = s.rdb.InvalidateLink(r.Context(), linkID)
	s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID, InstanceID: sc.Instance.InstanceID,
		Event: audit.PreviewLinkUpdated, ActorType: actorOf(sc), ActorID: actorID(sc),
		ResourceType: "preview_link", ResourceID: linkID, IP: httpx.ClientIP(r.Context())})
	httpx.JSON(w, http.StatusOK, map[string]any{"linkId": linkID, "updated": true})
}

// planMaxLinkTTL is the maximum link lifetime allowed by a plan; 0 means no cap
// beyond the backing file (spec §20: lite links/files are capped at 24h).
func planMaxLinkTTL(plan string) time.Duration {
	if plan == "lite" {
		return 24 * time.Hour
	}
	return 0
}

// linkCreateCap is the per-minute link-creation budget by trust level (spec §15.5).
func linkCreateCap(trust string) int {
	switch trust {
	case "trusted":
		return 120
	case "basic":
		return 60
	default: // new / unknown — conservative cold-start budget
		return 20
	}
}

// marshalPolicyForStorage serializes the policy, embedding the password hash
// (which has json:"-" on the public struct) so it persists.
func marshalPolicyForStorage(p store.AccessPolicy, plainPw string) json.RawMessage {
	type pwStore struct {
		Enabled      bool   `json:"enabled"`
		Hash         string `json:"hash"`
		AttemptLimit int    `json:"attemptLimit"`
	}
	type storable struct {
		Type          string   `json:"type"`
		AllowDownload bool     `json:"allowDownload"`
		IPAllowlist   []string `json:"ipAllowlist,omitempty"`
		MaxViews      int      `json:"maxViews,omitempty"`
		SingleUse     bool     `json:"singleUse,omitempty"`
		Password      *pwStore `json:"password,omitempty"`
		Account       any      `json:"account,omitempty"`
	}
	st := storable{Type: p.Type, AllowDownload: p.AllowDownload, IPAllowlist: p.IPAllowlist,
		MaxViews: p.MaxViews, SingleUse: p.SingleUse}
	if p.Password != nil {
		st.Password = &pwStore{Enabled: p.Password.Enabled, Hash: p.Password.Hash, AttemptLimit: p.Password.AttemptLimit}
	}
	if p.Account != nil {
		st.Account = p.Account
	}
	b, _ := json.Marshal(st)
	return b
}

// handleListLinks lists a tenant's links without secrets (spec §14).
func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "viewer")
	if sc == nil {
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	q := r.URL.Query()
	items, err := s.db.ListLinks(r.Context(), sc.TenantID, p, q.Get("status"), q.Get("mode"), q.Get("instanceId"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	// Redact the password hash from policy before returning (spec §14/UI §4.3).
	for i := range items {
		items[i].AccessPolicy = redactPolicy(items[i].AccessPolicy)
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(l store.PreviewLink) (time.Time, string) {
		return l.CreatedAt, l.LinkID
	}))
}

// handleRevokeLink revokes a link; cache invalidated for <=5s effect (spec §14/§19.7).
func (s *Server) handleRevokeLink(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	linkID := chi.URLParam(r, "id")
	t, err := s.db.RevokeLink(r.Context(), sc.TenantID, linkID)
	if err == store.ErrNotFound {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	_ = s.rdb.InvalidateLink(r.Context(), linkID)
	s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID,
		Event: audit.PreviewLinkRevoked, ActorType: actorOf(sc), ActorID: actorID(sc),
		ResourceType: "preview_link", ResourceID: linkID})
	httpx.JSON(w, http.StatusOK, map[string]any{"linkId": linkID, "revokedAt": t})
}

// actorOf/actorID map a data scope to audit actor fields (spec §15).
func actorOf(sc *dataScope) string {
	if sc.ViaKey {
		return audit.ActorInstanceAPIKey
	}
	return audit.ActorUser
}
func actorID(sc *dataScope) string {
	if sc.ViaKey && sc.Instance != nil {
		return sc.Instance.InstanceID
	}
	return sc.UserID
}

// --- small response helpers for idempotent closures ---

func badReq(r *http.Request, msg string) (int, any) {
	return http.StatusBadRequest, httpx.ErrorEnvelope{Error: httpx.ErrorBody{Code: httpx.CodeBadRequest, Message: msg, RequestID: httpx.RequestID(r.Context())}}
}
func notFound(r *http.Request) (int, any) {
	return http.StatusNotFound, httpx.ErrorEnvelope{Error: httpx.ErrorBody{Code: httpx.CodeNotFound, Message: "resource not found", RequestID: httpx.RequestID(r.Context())}}
}
func conflict(r *http.Request, msg string) (int, any) {
	return http.StatusConflict, httpx.ErrorEnvelope{Error: httpx.ErrorBody{Code: httpx.CodeConflict, Message: msg, RequestID: httpx.RequestID(r.Context())}}
}
func quotaBody(r *http.Request, msg string) (int, any) {
	return http.StatusForbidden, httpx.ErrorEnvelope{Error: httpx.ErrorBody{Code: httpx.CodeQuotaExceeded, Message: msg, RequestID: httpx.RequestID(r.Context())}}
}
func internalBody(r *http.Request) any {
	return httpx.ErrorEnvelope{Error: httpx.ErrorBody{Code: httpx.CodeInternal, Message: "internal error", RequestID: httpx.RequestID(r.Context()), Retryable: true}}
}
