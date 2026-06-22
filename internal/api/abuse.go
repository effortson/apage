package api

import (
	"net/http"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
)

type abuseReq struct {
	LinkID   string `json:"linkId"`
	Category string `json:"category"`
	Detail   string `json:"detail"`
}

var abuseCategories = map[string]bool{"phishing": true, "malware": true, "illegal": true, "other": true}

// handleAbuseReport accepts an anonymous abuse report (spec §30 / §15.5).
// No auth; rate-limited per source IP to prevent flooding.
func (s *Server) handleAbuseReport(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "abuse:"+httpx.ClientIP(r.Context()), 20, time.Hour) {
		return
	}
	var req abuseReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	if !abuseCategories[req.Category] {
		req.Category = "other"
	}
	if len(req.Detail) > 4000 {
		req.Detail = req.Detail[:4000]
	}
	var tenantID string
	if req.LinkID != "" {
		if l, err := s.db.LinkByID(r.Context(), req.LinkID); err == nil {
			tenantID = l.TenantID
		}
	}
	rep := store.AbuseReport{
		ReportID: id.New(id.PrefixReport), LinkID: req.LinkID, TenantID: tenantID,
		Category: req.Category, Detail: req.Detail, SourceIP: httpx.ClientIP(r.Context()),
	}
	if err := s.db.CreateAbuseReport(r.Context(), rep); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: tenantID, Event: audit.AbuseReported,
		ActorType: audit.ActorAnonymous, ResourceType: "preview_link", ResourceID: req.LinkID,
		IP: httpx.ClientIP(r.Context()), Reason: req.Category})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"reportId": rep.ReportID, "received": true})
}
