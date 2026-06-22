package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

type createInstanceReq struct {
	AgentType string `json:"agentType"`
	AgentName string `json:"agentName"`
	Mode      string `json:"mode"`
	Subdomain string `json:"subdomain"`
}

// handleCreateInstance provisions a cloud instance and issues its instance_api_key
// (plaintext returned once; hash stored). The key configures the agent's apage-cli
// MCP server, which is the only thing that can create preview links. Spec §26.
func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	var req createInstanceReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	if req.AgentType != "openclaw" && req.AgentType != "hermes" && req.AgentType != "custom" {
		httpx.BadRequest(w, r, "agentType must be openclaw|hermes|custom")
		return
	}
	if req.Mode == "" {
		req.Mode = "cloud"
	}
	if req.Mode != "cloud" {
		httpx.BadRequest(w, r, "mode must be cloud")
		return
	}
	if !validSubdomain(req.Subdomain) {
		httpx.BadRequest(w, r, "invalid or reserved subdomain")
		return
	}
	if req.AgentName == "" {
		req.AgentName = req.Subdomain
	}

	s.idempotent(au.TenantID, "create-instance", bodyHash(req), w, r, func() (int, any) {
		apiKey := id.NewSecret(id.SecretInstanceKey)
		in := store.Instance{
			InstanceID: id.New(id.PrefixInstance), TenantID: au.TenantID,
			AgentType: req.AgentType, AgentName: req.AgentName, Subdomain: req.Subdomain, Mode: req.Mode,
		}
		err := s.db.CreateInstance(r.Context(), in, hash.SecretHash(apiKey))
		switch {
		case errors.Is(err, store.ErrQuotaExceeded):
			return http.StatusForbidden, httpx.ErrorEnvelope{Error: httpx.ErrorBody{
				Code: httpx.CodeQuotaExceeded, Message: "instance_limit reached", RequestID: httpx.RequestID(r.Context())}}
		case err != nil:
			s.log.Error("create instance", "err", err)
			return http.StatusConflict, httpx.ErrorEnvelope{Error: httpx.ErrorBody{
				Code: httpx.CodeConflict, Message: "subdomain taken or invalid", RequestID: httpx.RequestID(r.Context())}}
		}
		s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: in.InstanceID,
			Event: audit.InstanceCreated, ActorType: audit.ActorUser, ActorID: au.UserID,
			ResourceType: "instance", ResourceID: in.InstanceID, IP: httpx.ClientIP(r.Context())})
		return http.StatusCreated, map[string]any{
			"instanceId":     in.InstanceID,
			"subdomain":      in.Subdomain,
			"url":            "https://" + in.Subdomain + "." + s.cfg.BaseDomain,
			"instanceApiKey": apiKey, // shown once (spec §26)
			"createdAt":      time.Now().UTC(),
		}
	})
}

// handleListInstances lists a tenant's instances (spec §26).
func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "viewer")
	if au == nil {
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	items, err := s.db.ListInstances(r.Context(), au.TenantID, p)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(i store.Instance) (time.Time, string) {
		return i.CreatedAt, i.InstanceID
	}))
}

// handleGetInstance returns instance detail (spec §26). In cloud-only mode an
// instance is a subdomain + API-key namespace with no live agent connection.
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "viewer")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"instance": in})
}

// handleDeleteInstance deletes an instance (cascade revokes links). Spec §26.
func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	if err := s.db.DeleteInstance(r.Context(), in.InstanceID); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: in.InstanceID,
		Event: "instance.deleted", ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "instance", ResourceID: in.InstanceID})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRotateCredentials issues a new instance api key, old key kept in grace (spec §26).
func (s *Server) handleRotateCredentials(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	apiKey := id.NewSecret(id.SecretInstanceKey)
	if err := s.db.RotateCredentials(r.Context(), in.InstanceID, hash.SecretHash(apiKey)); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: in.InstanceID,
		Event: audit.CredentialsRotated, ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "instance", ResourceID: in.InstanceID})
	httpx.JSON(w, http.StatusOK, map[string]any{"instanceApiKey": apiKey})
}

// loadOwnedInstance loads an instance and enforces tenant ownership (404 cross-tenant).
func (s *Server) loadOwnedInstance(w http.ResponseWriter, r *http.Request, au *authUser) (*store.Instance, error) {
	in, err := s.db.InstanceByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil || in.TenantID != au.TenantID {
		httpx.NotFound(w, r)
		return nil, err
	}
	return in, nil
}
