package api

import (
	"context"
	"net/http"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Moderation endpoints let an owner/admin quarantine abusive content (spec
// §15.5 disposition tiers: freeze link -> freeze instance). Distinct from
// revoke: a freeze is a reversible abuse hold that surfaces an appeal path. The
// runtime already honors frozen_at (runtime.go admitLink); these handlers are
// what finally write it, so the abuse machinery is live rather than dead code.

type reasonReq struct {
	Reason string `json:"reason"`
}

func optionalReason(r *http.Request) string {
	var body reasonReq
	_ = httpx.DecodeJSON(r, &body)
	if len(body.Reason) > 500 {
		body.Reason = body.Reason[:500]
	}
	return body.Reason
}

// handleFreezeLink freezes one of the tenant's preview links (spec §15.5).
func (s *Server) handleFreezeLink(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	linkID := chi.URLParam(r, "id")
	ok, err := s.db.FreezeLink(r.Context(), au.TenantID, linkID, optionalReason(r))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	_ = s.rdb.InvalidateLink(r.Context(), linkID) // <=5s effect (spec §19.7)
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: audit.LinkFrozen,
		ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "preview_link", ResourceID: linkID})
	httpx.JSON(w, http.StatusOK, map[string]any{"linkId": linkID, "frozen": true})
}

// handleUnfreezeLink lifts a link freeze (appeal resolved, spec §15.5).
func (s *Server) handleUnfreezeLink(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	linkID := chi.URLParam(r, "id")
	ok, err := s.db.UnfreezeLink(r.Context(), au.TenantID, linkID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	_ = s.rdb.InvalidateLink(r.Context(), linkID)
	httpx.JSON(w, http.StatusOK, map[string]any{"linkId": linkID, "frozen": false})
}

// handleFreezeInstance freezes a tenant instance and its links (spec §15.5).
func (s *Server) handleFreezeInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	instanceID := chi.URLParam(r, "id")
	ok, err := s.db.FreezeInstance(r.Context(), au.TenantID, instanceID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	// Freeze the instance's links so the runtime returns 410 immediately
	// (security review #1).
	_, _ = s.db.FreezeInstanceLinks(r.Context(), au.TenantID, instanceID, instanceFrozenReason)
	s.invalidateInstanceLinks(r.Context(), instanceID)
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: instanceID,
		Event: audit.InstanceFrozen, ActorType: audit.ActorUser, ActorID: au.UserID,
		ResourceType: "instance", ResourceID: instanceID})
	httpx.JSON(w, http.StatusOK, map[string]any{"instanceId": instanceID, "frozen": true})
}

// instanceFrozenReason tags links frozen as a side effect of an instance freeze
// so unfreezing the instance lifts only those (not independent abuse freezes).
const instanceFrozenReason = "instance_frozen"

func (s *Server) invalidateInstanceLinks(ctx context.Context, instanceID string) {
	if ids, err := s.db.InstanceLinkIDs(ctx, instanceID); err == nil {
		for _, lid := range ids {
			_ = s.rdb.InvalidateLink(ctx, lid)
		}
	}
}

// handleUnfreezeInstance lifts an instance freeze (spec §15.5).
func (s *Server) handleUnfreezeInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	instanceID := chi.URLParam(r, "id")
	ok, err := s.db.UnfreezeInstance(r.Context(), au.TenantID, instanceID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	// Lift only the instance-induced link freeze; independent abuse freezes stay.
	_, _ = s.db.UnfreezeInstanceLinks(r.Context(), au.TenantID, instanceID, instanceFrozenReason)
	s.invalidateInstanceLinks(r.Context(), instanceID)
	httpx.JSON(w, http.StatusOK, map[string]any{"instanceId": instanceID, "frozen": false})
}
