package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
)

const sessionCookie = "apage_session"

// csrfCookie holds a double-submit CSRF token. It is readable by the console JS
// (not HttpOnly) so the client can echo it in the X-CSRF-Token header (spec §25
// CSRF defense). A cross-site attacker can neither read the cookie nor set the
// custom header, so a forged request fails the comparison.
const csrfCookie = "apage_csrf"

type authCtxKey int

const (
	ctxUser authCtxKey = iota
	ctxMembership
	ctxInstance
	ctxScope
)

// dataScope is the unified tenant scope for data-plane endpoints, set by either
// an instance api key (agent/tool) or a console session (spec §14: lists serve
// both the admin console and the instance side).
type dataScope struct {
	TenantID string
	Instance *store.Instance // non-nil when authed by instance key
	UserID   string
	Role     string // effective role (instance key => admin-equivalent)
	ViaKey   bool
}

func scopeFrom(ctx context.Context) *dataScope {
	if v, ok := ctx.Value(ctxScope).(*dataScope); ok {
		return v
	}
	return nil
}

// idemScope isolates idempotency keys by tenant + instance (or session) so two
// callers cannot collide on the same key (spec §"幂等": tenant+instance+endpoint).
func (sc *dataScope) idemScope() string {
	if sc.ViaKey && sc.Instance != nil {
		return sc.TenantID + ":" + sc.Instance.InstanceID
	}
	return sc.TenantID + ":session"
}

// authUser carries the authenticated user + active tenant membership.
type authUser struct {
	UserID     string
	TenantID   string
	Role       string
	Membership *store.Membership
}

func userFrom(ctx context.Context) *authUser {
	if v, ok := ctx.Value(ctxUser).(*authUser); ok {
		return v
	}
	return nil
}

func instanceFrom(ctx context.Context) *store.Instance {
	if v, ok := ctx.Value(ctxInstance).(*store.Instance); ok {
		return v
	}
	return nil
}

// requireSession authenticates a console user via session cookie and resolves
// the active tenant from the X-Tenant-Id header (spec §2 RBAC chain:
// 登录态 -> Membership -> role).
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			httpx.Unauthorized(w, r, "no session")
			return
		}
		userID, err := s.db.SessionUser(r.Context(), c.Value)
		if err != nil {
			httpx.Unauthorized(w, r, "invalid or expired session")
			return
		}
		tenantID := r.Header.Get("X-Tenant-Id")
		au := &authUser{UserID: userID}
		if tenantID != "" {
			m, err := s.db.MembershipFor(r.Context(), userID, tenantID)
			if err != nil {
				httpx.Forbidden(w, r, "not a member of tenant")
				return
			}
			au.TenantID = tenantID
			au.Role = m.Role
			au.Membership = m
		}
		ctx := context.WithValue(r.Context(), ctxUser, au)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// issueCSRFToken sets a fresh double-submit CSRF cookie and returns its value
// (called on login/register/session so any active console gets a token).
func issueCSRFToken(w http.ResponseWriter, ttlSeconds int) string {
	tok := id.New("csrf_")
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: tok, Path: "/",
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: ttlSeconds, // not HttpOnly: JS must read it
	})
	return tok
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// csrfGuard enforces double-submit CSRF on cookie-authenticated, state-changing
// requests. Bearer (instance-key) callers and safe methods are exempt (spec §25).
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || bearer(r) != "" {
			next.ServeHTTP(w, r)
			return
		}
		// Only requests carrying a session cookie are CSRF-eligible.
		if _, err := r.Cookie(sessionCookie); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(csrfCookie)
		hdr := r.Header.Get("X-CSRF-Token")
		if err != nil || hdr == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(hdr)) != 1 {
			httpx.Forbidden(w, r, "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireTenant ensures an active tenant context is present.
func requireTenant(w http.ResponseWriter, r *http.Request) *authUser {
	au := userFrom(r.Context())
	if au == nil || au.TenantID == "" {
		httpx.BadRequest(w, r, "missing X-Tenant-Id header")
		return nil
	}
	return au
}

// roleRank orders roles for RBAC checks (spec §2 角色权限).
var roleRank = map[string]int{"viewer": 0, "member": 1, "admin": 2, "owner": 3}

// requireRole returns the active user if their role meets the minimum, else 403.
func requireRole(w http.ResponseWriter, r *http.Request, min string) *authUser {
	au := requireTenant(w, r)
	if au == nil {
		return nil
	}
	if roleRank[au.Role] < roleRank[min] {
		httpx.Forbidden(w, r, "insufficient role: requires "+min)
		return nil
	}
	return au
}

// requireInstanceKey authenticates a data-plane request via instance_api_key
// (spec §统一 API 约定: 实例侧 API 使用 instance_api_key).
func (s *Server) requireInstanceKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := bearer(r)
		if key == "" {
			httpx.Unauthorized(w, r, "missing instance api key")
			return
		}
		in, err := s.db.VerifyInstanceAPIKey(r.Context(), hash.SecretHash(key))
		if err != nil {
			httpx.Unauthorized(w, r, "invalid instance api key")
			return
		}
		ctx := context.WithValue(r.Context(), ctxInstance, in)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// requireData authenticates a data-plane request via instance api key OR console
// session, setting a unified dataScope (spec §14).
func (s *Server) requireData(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := bearer(r); key != "" {
			in, err := s.db.VerifyInstanceAPIKey(r.Context(), hash.SecretHash(key))
			if err != nil {
				httpx.Unauthorized(w, r, "invalid instance api key")
				return
			}
			sc := &dataScope{TenantID: in.TenantID, Instance: in, Role: "admin", ViaKey: true}
			ctx := context.WithValue(r.Context(), ctxInstance, in)
			ctx = context.WithValue(ctx, ctxScope, sc)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// Fall back to console session.
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			httpx.Unauthorized(w, r, "no credentials")
			return
		}
		userID, err := s.db.SessionUser(r.Context(), c.Value)
		if err != nil {
			httpx.Unauthorized(w, r, "invalid or expired session")
			return
		}
		tenantID := r.Header.Get("X-Tenant-Id")
		if tenantID == "" {
			httpx.BadRequest(w, r, "missing X-Tenant-Id header")
			return
		}
		m, err := s.db.MembershipFor(r.Context(), userID, tenantID)
		if err != nil {
			httpx.Forbidden(w, r, "not a member of tenant")
			return
		}
		sc := &dataScope{TenantID: tenantID, UserID: userID, Role: m.Role}
		ctx := context.WithValue(r.Context(), ctxScope, sc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// scopeRole enforces a minimum role for session-scoped data access (instance
// keys bypass since they are inherently authorized for their tenant).
func scopeRole(w http.ResponseWriter, r *http.Request, min string) *dataScope {
	sc := scopeFrom(r.Context())
	if sc == nil {
		httpx.Unauthorized(w, r, "no scope")
		return nil
	}
	if !sc.ViaKey && roleRank[sc.Role] < roleRank[min] {
		httpx.Forbidden(w, r, "insufficient role: requires "+min)
		return nil
	}
	return sc
}

// idemRecord is the stored idempotency entry: a hash of the original request
// plus the cached response body.
type idemRecord struct {
	Hash string `json:"h"`
	Body string `json:"b"`
}

// bodyHash returns a stable hash of a parsed request payload, used to detect a
// reused Idempotency-Key carrying a different request (spec §"幂等" / P1-3).
func bodyHash(v any) string {
	b, _ := json.Marshal(v)
	return hash.SecretHash(string(b))
}

// idempotent wraps a write handler so the same Idempotency-Key returns the cached
// response within 24h (spec §"幂等"). Scoped by tenant/instance + endpoint; a key
// reused with a different request body returns 409 rather than the stale result.
func (s *Server) idempotent(scopeID, endpoint, reqHash string, w http.ResponseWriter, r *http.Request, fn func() (int, any)) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		status, body := fn()
		httpx.JSON(w, status, body)
		return
	}
	scope := scopeID + ":" + endpoint + ":" + key
	if cached, ok, _ := s.rdb.IdempotencyGet(r.Context(), scope); ok {
		var rec idemRecord
		_ = json.Unmarshal([]byte(cached), &rec)
		if rec.Hash != "" && reqHash != "" && rec.Hash != reqHash {
			httpx.Conflict(w, r, "Idempotency-Key was already used with a different request")
			return
		}
		w.Header().Set("Idempotency-Replayed", "true")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(rec.Body))
		return
	}
	status, body := fn()
	if status >= 200 && status < 300 {
		if b := mustJSON(body); b != "" {
			rec, _ := json.Marshal(idemRecord{Hash: reqHash, Body: b})
			_ = s.rdb.IdempotencySet(r.Context(), scope, string(rec))
		}
	}
	httpx.JSON(w, status, body)
}
