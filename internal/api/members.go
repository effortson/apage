package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// handleListMembers lists members of the active tenant (spec §27).
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "member")
	if au == nil {
		return
	}
	members, err := s.db.ListMembers(r.Context(), au.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if members == nil {
		members = []store.Membership{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": members})
}

type inviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

var validRoles = map[string]bool{"owner": true, "admin": true, "member": true, "viewer": true}

// handleInviteMember creates an invite token and emails it (spec §27).
func (s *Server) handleInviteMember(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	var req inviteReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) || !validRoles[req.Role] {
		httpx.BadRequest(w, r, "invalid email or role")
		return
	}
	// admin cannot grant owner (spec §27).
	if req.Role == "owner" && au.Role != "owner" {
		httpx.Forbidden(w, r, "only owner can invite owners")
		return
	}
	s.idempotent(au.TenantID, "invite-member", bodyHash(req), w, r, func() (int, any) {
		tok := id.NewSecret("aps_")
		if err := s.db.CreateAuthToken(r.Context(), hash.SecretHash(tok), "", au.TenantID, "invite", req.Email, req.Role, time.Now().Add(7*24*time.Hour)); err != nil {
			return 500, internalBody(r)
		}
		_ = s.mail.Send(req.Email, "You're invited to an APAGE tenant", "Invite token: "+tok)
		s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: audit.MemberInvited,
			ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "member", Reason: req.Email})
		return http.StatusOK, map[string]bool{"ok": true}
	})
}

type acceptReq struct {
	Token string `json:"token"`
}

// handleAcceptInvite consumes an invite token; the caller must be logged in (spec §27).
func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		httpx.Unauthorized(w, r, "login required to accept invite")
		return
	}
	userID, err := s.db.SessionUser(r.Context(), c.Value)
	if err != nil {
		httpx.Unauthorized(w, r, "invalid session")
		return
	}
	var req acceptReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	row, err := s.db.ConsumeAuthToken(r.Context(), hash.SecretHash(req.Token), "invite")
	if err != nil {
		httpx.BadRequest(w, r, "invalid or expired invite")
		return
	}
	if err := s.db.CreateMembership(r.Context(), store.Membership{
		MembershipID: id.New(id.PrefixMembership), UserID: userID, TenantID: row.TenantID, Role: row.Role,
	}); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tenantId": row.TenantID, "role": row.Role})
}

type updateMemberReq struct {
	Role string `json:"role"`
}

// handleUpdateMember changes a member's role, preserving at least one owner (spec §27).
func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	var req updateMemberReq
	if err := httpx.DecodeJSON(r, &req); err != nil || !validRoles[req.Role] {
		httpx.BadRequest(w, r, "invalid role")
		return
	}
	m, err := s.db.MembershipByID(r.Context(), chi.URLParam(r, "membershipId"))
	if err != nil || m.TenantID != au.TenantID {
		httpx.NotFound(w, r)
		return
	}
	if m.Role == "owner" && req.Role != "owner" {
		if n, _ := s.db.CountOwners(r.Context(), au.TenantID); n <= 1 {
			httpx.Conflict(w, r, "cannot demote the last owner")
			return
		}
	}
	if req.Role == "owner" && au.Role != "owner" {
		httpx.Forbidden(w, r, "only owner can promote to owner")
		return
	}
	if err := s.db.UpdateMemberRole(r.Context(), m.MembershipID, req.Role); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: audit.MemberRoleChanged,
		ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "member", ResourceID: m.MembershipID, Reason: req.Role})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRemoveMember removes a member, preserving at least one owner (spec §27).
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	m, err := s.db.MembershipByID(r.Context(), chi.URLParam(r, "membershipId"))
	if err != nil || m.TenantID != au.TenantID {
		httpx.NotFound(w, r)
		return
	}
	if m.Role == "owner" {
		if n, _ := s.db.CountOwners(r.Context(), au.TenantID); n <= 1 {
			httpx.Conflict(w, r, "cannot remove the last owner")
			return
		}
	}
	if err := s.db.DeleteMembership(r.Context(), m.MembershipID); err != nil {
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: au.TenantID, Event: audit.MemberRemoved,
		ActorType: audit.ActorUser, ActorID: au.UserID, ResourceType: "member", ResourceID: m.MembershipID})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
