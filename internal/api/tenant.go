package api

import (
	"net/http"
	"strings"

	"github.com/apage/apage/internal/httpx"
)

type updateTenantReq struct {
	Name string `json:"name"`
}

// handleUpdateTenant renames the active tenant (settings profile, UI §7.9).
func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	var req updateTenantReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 200 {
		httpx.BadRequest(w, r, "name must be 1–200 characters")
		return
	}
	if err := s.db.UpdateTenantName(r.Context(), au.TenantID, name); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}
