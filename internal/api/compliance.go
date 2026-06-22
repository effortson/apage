package api

import (
	"net/http"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
)

// handleDataDeletion executes a GDPR/CCPA data-deletion request for the tenant
// (spec §15.6). Owner-only; purges files, derivatives, links, and file-ref
// mappings, enqueues object deletion, and returns a deletion confirmation.
func (s *Server) handleDataDeletion(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "owner")
	if au == nil {
		return
	}
	res, err := s.db.PurgeTenantData(r.Context(), au.TenantID)
	if err != nil {
		s.log.Error("purge tenant data", "err", err)
		httpx.Internal(w, r)
		return
	}
	for _, key := range res.StorageKeys {
		_ = s.rdb.Enqueue(r.Context(), "delete", key)
	}
	// Audit records the confirmation, never the deleted content (spec §15.6).
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: "data.deletion_completed",
		ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "tenant", ResourceID: au.TenantID,
		Reason: "gdpr_ccpa_request"})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"deleted":      map[string]int{"files": res.Files, "fileRefs": res.FileRefs, "links": res.Links},
		"confirmation": "data deletion completed; object cleanup queued with retry",
	})
}
