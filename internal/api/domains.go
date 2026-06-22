package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// handleListDomains lists custom domains (spec §28).
func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	items, err := s.db.ListDomains(r.Context(), au.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if items == nil {
		items = []store.CustomDomain{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type createDomainReq struct {
	Domain string `json:"domain"`
}

// handleCreateDomain adds a custom domain and returns TXT + CNAME records (spec §28).
func (s *Server) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	var req createDomainReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" || !strings.Contains(req.Domain, ".") {
		httpx.BadRequest(w, r, "invalid domain")
		return
	}
	s.idempotent(au.TenantID, "create-domain", bodyHash(req), w, r, func() (int, any) {
		d := store.CustomDomain{
			DomainID: id.New(id.PrefixDomain), TenantID: au.TenantID, Domain: req.Domain,
			TXTValue: "apage-domain-verify=" + id.NewSecret("")[:24],
		}
		err := s.db.CreateDomain(r.Context(), d)
		switch {
		case errors.Is(err, store.ErrQuotaExceeded):
			return quotaBody(r, "custom_domain_limit reached")
		case err != nil:
			return conflict(r, "domain already added or invalid")
		}
		return http.StatusCreated, map[string]any{
			"domainId": d.DomainID, "domain": d.Domain, "status": "pending",
			"dns": map[string]any{
				"txt":   map[string]string{"name": "_apage." + d.Domain, "value": d.TXTValue},
				"cname": map[string]string{"name": d.Domain, "value": au.TenantID + "." + s.cfg.BaseDomain},
			},
		}
	})
}

// handleGetDomain returns domain detail with expected-vs-observed DNS (spec §28).
func (s *Server) handleGetDomain(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	d, err := s.db.DomainByID(r.Context(), au.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"domain": d,
		"expectedDns": map[string]any{
			"txt":   map[string]string{"name": "_apage." + d.Domain, "value": d.TXTValue},
			"cname": map[string]string{"name": d.Domain, "value": au.TenantID + "." + s.cfg.BaseDomain},
		},
	})
}

// handleVerifyDomain triggers a DNS check + ACME issuance (spec §28).
// MVP performs a best-effort TXT lookup; full ACME is a V1 worker concern.
func (s *Server) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	d, err := s.db.DomainByID(r.Context(), au.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	verified := s.checkDomainTXT(d.Domain, d.TXTValue)
	status, cert := "failed", "failed"
	event := audit.CustomDomainFailed
	if verified {
		status, cert, event = "verified", "issued", audit.CustomDomainVerified
	}
	_ = s.db.SetDomainStatus(r.Context(), d.DomainID, status, cert)
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: event,
		ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "custom_domain", ResourceID: d.DomainID})
	httpx.JSON(w, http.StatusOK, map[string]any{"domainId": d.DomainID, "status": status, "certStatus": cert})
}

// handleDeleteDomain removes a custom domain (spec §28).
func (s *Server) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	if err := s.db.DeleteDomain(r.Context(), au.TenantID, chi.URLParam(r, "id")); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
