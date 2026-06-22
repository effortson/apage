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
)

const sessionTTL = 14 * 24 * time.Hour

type registerReq struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantName string `json:"tenantName"`
}

// handleRegister atomically creates User + Tenant(lite/new) + owner Membership +
// Quota (spec §25). Rate-limited per source IP to slow abuse.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "register:"+httpx.ClientIP(r.Context()), 10, time.Hour) {
		return
	}
	var req registerReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(req.Email) {
		httpx.BadRequest(w, r, "invalid email")
		return
	}
	if !strongPassword(req.Password) {
		httpx.BadRequest(w, r, "password must be >=10 chars with letters and digits")
		return
	}
	if _, err := s.db.UserByEmail(r.Context(), req.Email); err == nil {
		httpx.Conflict(w, r, "email already registered")
		return
	}
	ph, err := hash.Password(req.Password)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	tenantName := req.TenantName
	if tenantName == "" {
		tenantName = req.Email
	}
	tenant := store.Tenant{TenantID: id.New(id.PrefixTenant), Name: tenantName, Plan: "lite", TrustLevel: "new"}
	user := store.User{UserID: id.New(id.PrefixUser), Email: req.Email, AuthProvider: "password", PasswordHash: ph}
	if err := s.db.RegisterAccount(r.Context(), tenant, user, id.New(id.PrefixMembership)); err != nil {
		s.log.Error("register", "err", err)
		httpx.Internal(w, r)
		return
	}
	s.audit(r.Context(), audit.Entry{TenantID: tenant.TenantID, Event: audit.UserRegistered,
		ActorType: audit.ActorUser, ActorID: user.UserID, IP: httpx.ClientIP(r.Context())})
	s.sendVerification(r, user.UserID, user.Email)
	s.startSession(w, r, user.UserID)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"userId": user.UserID, "tenantId": tenant.TenantID, "email": user.Email,
	})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin verifies credentials with per-IP+account rate limiting (spec §25).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !s.limitStrict(w, r, "login:"+httpx.ClientIP(r.Context())+":"+req.Email, 10, 15*time.Minute) {
		return
	}
	u, err := s.db.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httpx.Unauthorized(w, r, "invalid credentials")
		return
	}
	ok, _ := hash.VerifyPassword(req.Password, u.PasswordHash)
	if !ok {
		httpx.Unauthorized(w, r, "invalid credentials")
		return
	}
	s.startSession(w, r, u.UserID)
	httpx.JSON(w, http.StatusOK, map[string]any{"userId": u.UserID, "email": u.Email})
}

// handleLogout deletes the current session (spec §25).
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.db.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", Domain: s.cookieDomain(), MaxAge: -1, HttpOnly: true})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleSession returns the current user + accessible tenants (spec §25).
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	au := userFrom(r.Context())
	u, err := s.db.UserByID(r.Context(), au.UserID)
	if err != nil {
		httpx.Unauthorized(w, r, "user gone")
		return
	}
	// Refresh the CSRF token so any already-active session obtains one.
	issueCSRFToken(w, s.cookieDomain(), int(sessionTTL.Seconds()))
	ms, _ := s.db.MembershipsForUser(r.Context(), au.UserID)
	tenants := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		t, err := s.db.TenantByID(r.Context(), m.TenantID)
		if err != nil {
			continue
		}
		tenants = append(tenants, map[string]any{
			"tenantId": t.TenantID, "name": t.Name, "plan": t.Plan, "role": m.Role,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":    map[string]any{"userId": u.UserID, "email": u.Email, "emailVerified": u.EmailVerifiedAt != nil},
		"tenants": tenants,
	})
}

type tokenReq struct {
	Token string `json:"token"`
}

// handleVerifyEmail consumes a verification token (spec §25).
func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenReq
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Token == "" {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	row, err := s.db.ConsumeAuthToken(r.Context(), hash.SecretHash(req.Token), "verify_email")
	if err != nil {
		httpx.BadRequest(w, r, "invalid or expired token")
		return
	}
	_ = s.db.MarkEmailVerified(r.Context(), row.UserID)
	httpx.JSON(w, http.StatusOK, map[string]bool{"verified": true})
}

type emailReq struct {
	Email string `json:"email"`
}

// handleResendVerification re-sends a verification email (spec §25).
func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req emailReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Throttle per IP and per target email to prevent verification-email bombing
	// (security review #5).
	if !s.limit(w, r, "resend:"+httpx.ClientIP(r.Context()), 5, time.Hour) ||
		!s.limit(w, r, "resend:"+email, 5, time.Hour) {
		return
	}
	if u, err := s.db.UserByEmail(r.Context(), email); err == nil && u.EmailVerifiedAt == nil {
		s.sendVerification(r, u.UserID, u.Email)
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleForgotPassword always returns 200 to avoid account enumeration (spec §25).
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req emailReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Throttle per IP and per target email to prevent reset-email bombing and
	// token churn (security review #5).
	if !s.limit(w, r, "forgot:"+httpx.ClientIP(r.Context()), 5, time.Hour) ||
		!s.limit(w, r, "forgot:"+email, 5, time.Hour) {
		return
	}
	if u, err := s.db.UserByEmail(r.Context(), email); err == nil {
		tok := id.NewSecret("aps_")
		_ = s.db.CreateAuthToken(r.Context(), hash.SecretHash(tok), u.UserID, "", "reset_password", "", "", time.Now().Add(time.Hour))
		_ = s.mail.Send(u.Email, "Reset your APAGE password", "Reset token: "+tok)
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type resetReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handleResetPassword sets a new password from a reset token (spec §25).
func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	if !strongPassword(req.Password) {
		httpx.BadRequest(w, r, "weak password")
		return
	}
	row, err := s.db.ConsumeAuthToken(r.Context(), hash.SecretHash(req.Token), "reset_password")
	if err != nil {
		httpx.BadRequest(w, r, "invalid or expired token")
		return
	}
	ph, err := hash.Password(req.Password)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	_ = s.db.SetPassword(r.Context(), row.UserID, ph)
	// Revoke all existing sessions so a reset (e.g. account recovery) does not
	// leave a prior attacker session live (spec §25 / security review #5).
	_ = s.db.DeleteSessionsForUser(r.Context(), row.UserID)
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- session + email helpers ---

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string) {
	sid := id.New(id.PrefixSession)
	_ = s.db.CreateSession(r.Context(), sid, userID, time.Now().Add(sessionTTL))
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: sid, Path: "/", Domain: s.cookieDomain(), HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds()),
	})
	issueCSRFToken(w, s.cookieDomain(), int(sessionTTL.Seconds()))
}

func (s *Server) sendVerification(r *http.Request, userID, email string) {
	tok := id.NewSecret("aps_")
	_ = s.db.CreateAuthToken(r.Context(), hash.SecretHash(tok), userID, "", "verify_email", "", "", time.Now().Add(24*time.Hour))
	_ = s.mail.Send(email, "Verify your APAGE email", "Verification token: "+tok)
}

// limit applies a rate limit and writes a 429 if exceeded (spec §"限流响应约定").
// Fail-open on a limiter error so a transient Redis blip does not take down
// general traffic; sensitive credential checks use limitStrict instead.
func (s *Server) limit(w http.ResponseWriter, r *http.Request, key string, max int, window time.Duration) bool {
	return s.rateLimit(w, r, key, max, window, false)
}

// limitStrict is the fail-closed variant used for brute-force-sensitive
// credential endpoints (login, MFA, password unlock): if the limiter is
// unavailable, deny rather than allow unbounded attempts (security review #10).
func (s *Server) limitStrict(w http.ResponseWriter, r *http.Request, key string, max int, window time.Duration) bool {
	return s.rateLimit(w, r, key, max, window, true)
}

func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request, key string, max int, window time.Duration, failClosed bool) bool {
	res, err := s.rdb.RateLimit(r.Context(), key, max, window)
	if err != nil {
		s.log.Error("rate limiter error", "key", key, "failClosed", failClosed, "err", err)
		if failClosed {
			httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "rate limiter unavailable, try again shortly", true)
			return false
		}
		return true // fail open on limiter error
	}
	httpx.SetRateLimitHeaders(w, res.Limit, res.Remaining, res.ResetUnix)
	if !res.Allowed {
		httpx.TooManyRequests(w, r, int(time.Until(time.Unix(res.ResetUnix, 0)).Seconds())+1)
		return false
	}
	return true
}
