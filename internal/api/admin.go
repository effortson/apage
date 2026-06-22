package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/apage/apage/internal/totp"
	"github.com/go-chi/chi/v5"
)

const (
	adminSessionCookie = "apage_admin_session"
	adminPendingCookie = "apage_admin_pending"
	adminSessionTTL    = 8 * time.Hour
	adminPendingTTL    = 5 * time.Minute
)

type adminCtxKey int

const ctxAdmin adminCtxKey = 0

func adminFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAdmin).(string); ok {
		return v
	}
	return ""
}

// adminIPGate enforces the admin IP allowlist (spec §8 network isolation). An
// empty allowlist disables the check (dev only).
func (s *Server) adminIPGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.cfg.AdminIPAllowlist) > 0 && !ipInList(httpx.ClientIP(r.Context()), s.cfg.AdminIPAllowlist) {
			httpx.NotFound(w, r) // do not reveal the admin plane to disallowed IPs
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin authenticates a platform admin via the admin session cookie.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(adminSessionCookie)
		if err != nil || c.Value == "" {
			httpx.Unauthorized(w, r, "admin login required")
			return
		}
		adminID, ok, _ := s.rdb.GetKV(r.Context(), "adminsess:"+c.Value)
		if !ok {
			httpx.Unauthorized(w, r, "invalid or expired admin session")
			return
		}
		ctx := context.WithValue(r.Context(), ctxAdmin, adminID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ipInList(ip string, list []string) bool {
	parsed := net.ParseIP(ip)
	for _, c := range list {
		if _, network, err := net.ParseCIDR(c); err == nil {
			if parsed != nil && network.Contains(parsed) {
				return true
			}
		} else if c == ip {
			return true
		}
	}
	return false
}

// --- admin auth flow: password -> TOTP MFA (spec §8) ---

type adminLoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleAdminLogin verifies the password and starts the MFA challenge. On first
// login it returns a TOTP enrollment URI to scan.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !s.limitStrict(w, r, "adminlogin:"+httpx.ClientIP(r.Context()), 10, 15*time.Minute) {
		return
	}
	var req adminLoginReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	a, err := s.db.PlatformAdminByEmail(r.Context(), req.Email)
	if err != nil {
		hash.DummyVerify(req.Password) // equalize timing so a missing admin is indistinguishable
		httpx.Unauthorized(w, r, "invalid credentials")
		return
	}
	if ok, _ := hash.VerifyPassword(req.Password, a.PasswordHash); !ok {
		httpx.Unauthorized(w, r, "invalid credentials")
		return
	}

	// Issue a short-lived pending token bound to a cookie for the MFA step.
	pending := id.New("adp_")
	_ = s.rdb.SetKV(r.Context(), "adminpending:"+pending, a.AdminID, adminPendingTTL)
	http.SetCookie(w, &http.Cookie{
		Name: adminPendingCookie, Value: pending, Path: "/admin/v1/auth",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: int(adminPendingTTL.Seconds()),
	})

	resp := map[string]any{"mfaRequired": true, "enrolled": a.MFAEnrolled}
	if !a.MFAEnrolled {
		// Enrollment: mint a TOTP secret and return the otpauth URI to scan.
		secret, err := totp.GenerateSecret()
		if err != nil {
			httpx.Internal(w, r)
			return
		}
		if err := s.db.SetAdminTOTP(r.Context(), a.AdminID, secret); err != nil {
			httpx.Internal(w, r)
			return
		}
		resp["otpauthUri"] = totp.OtpauthURI(secret, a.Email, "APAGE Admin")
	}
	httpx.JSON(w, http.StatusOK, resp)
}

type adminMFAReq struct {
	Code string `json:"code"`
}

// handleAdminMFA verifies the TOTP code and issues the admin session.
func (s *Server) handleAdminMFA(w http.ResponseWriter, r *http.Request) {
	if !s.limitStrict(w, r, "adminmfa:"+httpx.ClientIP(r.Context()), 10, 15*time.Minute) {
		return
	}
	c, err := r.Cookie(adminPendingCookie)
	if err != nil || c.Value == "" {
		httpx.Unauthorized(w, r, "no pending login")
		return
	}
	adminID, ok, _ := s.rdb.GetKV(r.Context(), "adminpending:"+c.Value)
	if !ok {
		httpx.Unauthorized(w, r, "login challenge expired")
		return
	}
	var req adminMFAReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	a, err := s.db.PlatformAdminByID(r.Context(), adminID)
	if err != nil || a.TOTPSecret == "" {
		httpx.Unauthorized(w, r, "mfa not set up")
		return
	}
	step, ok := totp.VerifyStep(a.TOTPSecret, req.Code, time.Now())
	if !ok {
		httpx.Unauthorized(w, r, "invalid code")
		return
	}
	// Single-use per time-step: reject a code that was already accepted within its
	// validity window so a captured/observed code cannot be replayed (RFC 6238 §5.2).
	nonce := fmt.Sprintf("adminmfastep:%s:%d", a.AdminID, step)
	if fresh, err := s.rdb.SetNX(r.Context(), nonce, "1", 2*time.Minute); err == nil && !fresh {
		httpx.Unauthorized(w, r, "code already used")
		return
	}
	if !a.MFAEnrolled {
		_ = s.db.EnrollAdminMFA(r.Context(), a.AdminID) // confirm enrollment on first valid code
	}
	_ = s.rdb.DelKV(r.Context(), "adminpending:"+c.Value)
	http.SetCookie(w, &http.Cookie{Name: adminPendingCookie, Value: "", Path: "/admin/v1/auth", MaxAge: -1})

	token := id.New("ads_")
	_ = s.rdb.SetKV(r.Context(), "adminsess:"+token, a.AdminID, adminSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: token, Path: "/admin/v1",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: int(adminSessionTTL.Seconds()),
	})
	_ = s.db.TouchAdminLogin(r.Context(), a.AdminID)
	s.audit(r.Context(), audit.Entry{Event: audit.AdminLogin, ActorType: audit.ActorAdmin,
		ActorID: a.AdminID, IP: httpx.ClientIP(r.Context())})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminSessionCookie); err == nil {
		_ = s.rdb.DelKV(r.Context(), "adminsess:"+c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminSessionCookie, Value: "", Path: "/admin/v1", MaxAge: -1})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- admin operations (metadata only, all audited) ---

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	tenants, _ := s.db.CountTenants(r.Context())
	instances, _ := s.db.CountInstances(r.Context())
	links, _ := s.db.CountActiveLinks(r.Context())
	scanQ, _ := s.rdb.QueueLen(r.Context(), "scan")
	deleteQ, _ := s.rdb.QueueLen(r.Context(), "delete")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tenants":     tenants,
		"instances":   instances,
		"activeLinks": links,
		"queues":      map[string]int64{"scan": scanQ, "delete": deleteQ},
	})
}

func (s *Server) handleAdminListTenants(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	items, err := s.db.ListTenantsAdmin(r.Context(), p, r.URL.Query().Get("q"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(t store.Tenant) (time.Time, string) {
		return t.CreatedAt, t.TenantID
	}))
}

func (s *Server) handleAdminTenantDetail(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	t, err := s.db.TenantByID(r.Context(), tenantID)
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	q, _ := s.db.QuotaFor(r.Context(), tenantID)
	httpx.JSON(w, http.StatusOK, map[string]any{"tenant": t, "quota": q})
}

type adminTrustReq struct {
	Trust string `json:"trust"`
}

var validTrust = map[string]bool{"new": true, "basic": true, "trusted": true}

func (s *Server) handleAdminSetTrust(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	var req adminTrustReq
	if err := httpx.DecodeJSON(r, &req); err != nil || !validTrust[req.Trust] {
		httpx.BadRequest(w, r, "trust must be new|basic|trusted")
		return
	}
	ok, err := s.db.SetTenantTrust(r.Context(), tenantID, req.Trust)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: tenantID, Event: audit.TenantTrustChanged,
		ActorType: audit.ActorAdmin, ActorID: adminFrom(r.Context()), ResourceType: "tenant", ResourceID: tenantID, Reason: req.Trust})
	httpx.JSON(w, http.StatusOK, map[string]any{"tenantId": tenantID, "trust": req.Trust})
}

const suspendReason = "tenant_suspended"

func (s *Server) handleAdminSuspend(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	ok, err := s.db.SetTenantStatus(r.Context(), tenantID, "suspended")
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	// Teeth: freeze the tenant's live links (runtime returns 410) and invalidate
	// caches so it takes effect immediately (spec §15.5).
	_, _ = s.db.FreezeTenantLinks(r.Context(), tenantID, suspendReason)
	s.invalidateTenantLinks(r.Context(), tenantID)
	s.audit(r.Context(), audit.Entry{TenantID: tenantID, Event: audit.TenantSuspended,
		ActorType: audit.ActorAdmin, ActorID: adminFrom(r.Context()), ResourceType: "tenant", ResourceID: tenantID})
	httpx.JSON(w, http.StatusOK, map[string]any{"tenantId": tenantID, "status": "suspended"})
}

func (s *Server) handleAdminRestore(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	ok, err := s.db.SetTenantStatus(r.Context(), tenantID, "active")
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	// Only lift the suspension-induced freeze, leaving abuse freezes intact.
	_, _ = s.db.UnfreezeTenantLinks(r.Context(), tenantID, suspendReason)
	s.invalidateTenantLinks(r.Context(), tenantID)
	s.audit(r.Context(), audit.Entry{TenantID: tenantID, Event: audit.TenantRestored,
		ActorType: audit.ActorAdmin, ActorID: adminFrom(r.Context()), ResourceType: "tenant", ResourceID: tenantID})
	httpx.JSON(w, http.StatusOK, map[string]any{"tenantId": tenantID, "status": "active"})
}

func (s *Server) invalidateTenantLinks(ctx context.Context, tenantID string) {
	if ids, err := s.db.TenantLinkIDs(ctx, tenantID); err == nil {
		for _, lid := range ids {
			_ = s.rdb.InvalidateLink(ctx, lid)
		}
	}
}

func (s *Server) handleAdminListAbuse(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	items, err := s.db.ListAbuseReports(r.Context(), p, r.URL.Query().Get("status"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(a store.AbuseReport) (time.Time, string) {
		return a.CreatedAt, a.ReportID
	}))
}

type adminAbuseActionReq struct {
	Status string `json:"status"`
}

func (s *Server) handleAdminActionAbuse(w http.ResponseWriter, r *http.Request) {
	reportID := chi.URLParam(r, "id")
	var req adminAbuseActionReq
	if err := httpx.DecodeJSON(r, &req); err != nil || (req.Status != "actioned" && req.Status != "dismissed") {
		httpx.BadRequest(w, r, "status must be actioned|dismissed")
		return
	}
	ok, err := s.db.ActionAbuseReport(r.Context(), reportID, req.Status)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{Event: audit.AbuseActioned, ActorType: audit.ActorAdmin,
		ActorID: adminFrom(r.Context()), ResourceType: "abuse_report", ResourceID: reportID, Reason: req.Status})
	httpx.JSON(w, http.StatusOK, map[string]any{"reportId": reportID, "status": req.Status})
}

func (s *Server) handleAdminListAudit(w http.ResponseWriter, r *http.Request) {
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	q := r.URL.Query()
	items, err := s.db.ListAuditAll(r.Context(), p, q.Get("event"), q.Get("tenantId"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(a store.AuditLog) (time.Time, string) {
		return a.CreatedAt, a.EventID
	}))
}

// BootstrapAdmin seeds the first platform admin from config if none exist
// (spec §8). The admin must enroll TOTP on first login.
func (s *Server) BootstrapAdmin(ctx context.Context) {
	if s.cfg.AdminBootstrapEmail == "" || s.cfg.AdminBootstrapPassword == "" {
		return
	}
	if n, err := s.db.CountPlatformAdmins(ctx); err != nil || n > 0 {
		return
	}
	ph, err := hash.Password(s.cfg.AdminBootstrapPassword)
	if err != nil {
		s.log.Error("admin bootstrap hash", "err", err)
		return
	}
	a := store.PlatformAdmin{AdminID: id.New("adm_"), Email: strings.ToLower(s.cfg.AdminBootstrapEmail), PasswordHash: ph}
	if err := s.db.CreatePlatformAdmin(ctx, a); err != nil {
		s.log.Error("admin bootstrap create", "err", err)
		return
	}
	s.log.Info("seeded bootstrap platform admin", "email", a.Email)
}
