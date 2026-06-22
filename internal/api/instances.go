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

// handleCreateInstance provisions an instance and issues agent_token +
// instance_api_key (plaintext returned once; hashes stored). Spec §26.
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
		req.Mode = "tunnel"
	}
	if req.Mode != "tunnel" && req.Mode != "cloud" && req.Mode != "hybrid" {
		httpx.BadRequest(w, r, "mode must be tunnel|cloud|hybrid")
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
		agentToken := id.NewSecret(id.SecretAgentToken)
		apiKey := id.NewSecret(id.SecretInstanceKey)
		in := store.Instance{
			InstanceID: id.New(id.PrefixInstance), TenantID: au.TenantID,
			AgentType: req.AgentType, AgentName: req.AgentName, Subdomain: req.Subdomain, Mode: req.Mode,
		}
		err := s.db.CreateInstance(r.Context(), in, hash.SecretHash(agentToken), hash.SecretHash(apiKey))
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
			"agentToken":     agentToken, // shown once (spec §26)
			"instanceApiKey": apiKey,     // shown once
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

// handleGetInstance returns instance detail incl. live connection health (spec §26).
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "viewer")
	if au == nil {
		return
	}
	in, err := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	_ = err
	gw, sess, online, _ := s.rdb.LookupAgent(r.Context(), in.InstanceID)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"instance": in,
		"connection": map[string]any{
			"online": online, "gatewayId": gw, "sessionId": sess,
			"protocolVersion": s.cfg.AgentMinProtocolVersion,
		},
		// allowlist is reported by the agent; surfaced read-only (spec §6.3).
		"allowlist": map[string]any{"note": "allowlist is configured on the customer server and reported by the agent; the console cannot widen it remotely"},
	})
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

// handleRotateCredentials issues new credentials, old api key kept in grace (spec §26).
func (s *Server) handleRotateCredentials(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	agentToken := id.NewSecret(id.SecretAgentToken)
	apiKey := id.NewSecret(id.SecretInstanceKey)
	if err := s.db.RotateCredentials(r.Context(), in.InstanceID, hash.SecretHash(agentToken), hash.SecretHash(apiKey)); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: in.InstanceID,
		Event: audit.CredentialsRotated, ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "instance", ResourceID: in.InstanceID})
	httpx.JSON(w, http.StatusOK, map[string]any{"agentToken": agentToken, "instanceApiKey": apiKey})
}

// handleRevokeToken revokes the agent token and disconnects active sessions (spec §26).
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	if err := s.db.RevokeAgentToken(r.Context(), in.InstanceID); err != nil {
		httpx.Internal(w, r)
		return
	}
	// Drop the registry mapping so the gateway tears down the session (spec §26).
	_ = s.rdb.UnregisterAgent(r.Context(), in.InstanceID)
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, InstanceID: in.InstanceID,
		Event: audit.TokenRevoked, ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "instance", ResourceID: in.InstanceID})
	httpx.JSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// handleAllowlistChange generates a change instruction requiring on-host
// confirmation; the console never widens the allowlist remotely (spec §6.3/§26).
func (s *Server) handleAllowlistChange(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	in, _ := s.loadOwnedInstance(w, r, au)
	if in == nil {
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"requestId": id.New(id.PrefixRequest),
		"instruction": "Run on the customer server: apage-agent allowlist apply --request <id>. " +
			"The change must be confirmed on the host; the console cannot widen the allowlist remotely.",
	})
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
