// Package api implements the apage-api service: REST control plane, data-plane
// preview-link APIs, and the visitor-facing runtime endpoints (spec §8/§12/§14/
// §25–§31).
package api

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/apage/apage/internal/config"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// Server holds shared dependencies for all handlers.
type Server struct {
	cfg   *config.Config
	db    *store.Store
	rdb   *redisx.Client
	log   *slog.Logger
	mail  Mailer
	gw    GatewayClient
	store ObjectStore

	previewAccessTotal int64 // atomic counter for /metrics (spec §18)
}

// Mailer sends transactional email (verification, invites, resets).
type Mailer interface {
	Send(to, subject, body string) error
}

// GatewayClient streams a tunnel file via the gateway's internal endpoint.
type GatewayClient interface {
	// StreamFile asks the gateway at gatewayURL (resolved from the registry) to
	// stream fileRef to w, honoring the Range header. An empty gatewayURL uses the
	// client's configured fallback. Returns the upstream status.
	StreamFile(w http.ResponseWriter, r *http.Request, gatewayURL, instanceID, fileRef string) error
}

// ObjectStore abstracts cloud object storage (spec §11).
type ObjectStore interface {
	Put(key, contentType string, r io.Reader) error
	PresignPut(key, contentType string) (url string, headers map[string]string, err error)
	PresignGet(key, downloadName string) (string, error)
	Get(key string) (body io.ReadSeekCloser, contentType string, size int64, err error)
	Stat(key string) (size int64, etag string, err error)
	Delete(keys ...string) error
}

// New constructs a Server.
func New(cfg *config.Config, db *store.Store, rdb *redisx.Client, log *slog.Logger, mail Mailer, gw GatewayClient, obj ObjectStore) *Server {
	return &Server{cfg: cfg, db: db, rdb: rdb, log: log, mail: mail, gw: gw, store: obj}
}

// Router builds the full HTTP routing tree.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestContext(true)) // behind our edge: trust X-Forwarded-For
	r.Use(httpx.Recover(s.log))
	r.Use(httpx.Logger(s.log))

	// Platform/ops endpoints (spec §31).
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// Visitor runtime endpoints (spec §30) — no Bearer auth, secret in path.
	r.Get("/p/{linkId}/{secret}", s.handlePreview)
	r.Get("/p/{linkId}/{secret}/raw", s.handlePreviewRaw) // sandboxed active-content bytes (§15.5)
	r.Post("/p/{linkId}/{secret}/unlock", s.handleUnlock)
	r.Get("/f/{fileId}/{secret}", s.handleFileDirect)

	r.Route("/api/v1", func(r chi.Router) {
		// Auth (spec §25) — no auth required.
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/logout", s.handleLogout)
		r.Post("/auth/verify-email", s.handleVerifyEmail)
		r.Post("/auth/resend-verification", s.handleResendVerification)
		r.Post("/auth/forgot-password", s.handleForgotPassword)
		r.Post("/auth/reset-password", s.handleResetPassword)

		// OAuth (spec §25) — browser GET navigations; inert until a provider is
		// configured with a client id/secret.
		r.Get("/auth/providers", s.handleOAuthProviders)
		r.Get("/auth/oauth/{provider}/start", s.handleOAuthStart)
		r.Get("/auth/oauth/{provider}/callback", s.handleOAuthCallback)

		// Public abuse report (spec §30) — no auth.
		r.Post("/public/abuse-reports", s.handleAbuseReport)

		// Session-authenticated control plane (spec §26–§29).
		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Use(s.csrfGuard)
			r.Get("/auth/session", s.handleSession)

			r.Post("/instances", s.handleCreateInstance)
			r.Get("/instances", s.handleListInstances)
			r.Get("/instances/{id}", s.handleGetInstance)
			r.Delete("/instances/{id}", s.handleDeleteInstance)
			r.Post("/instances/{id}/rotate-credentials", s.handleRotateCredentials)
			r.Post("/instances/{id}/revoke-token", s.handleRevokeToken)
			r.Post("/instances/{id}/allowlist-change-request", s.handleAllowlistChange)
			r.Post("/instances/{id}/freeze", s.handleFreezeInstance)
			r.Post("/instances/{id}/unfreeze", s.handleUnfreezeInstance)

			// Abuse moderation (spec §15.5) — freeze/unfreeze links.
			r.Post("/preview-links/{id}/freeze", s.handleFreezeLink)
			r.Post("/preview-links/{id}/unfreeze", s.handleUnfreezeLink)

			r.Get("/members", s.handleListMembers)
			r.Post("/members/invite", s.handleInviteMember)
			r.Patch("/members/{membershipId}", s.handleUpdateMember)
			r.Delete("/members/{membershipId}", s.handleRemoveMember)

			r.Get("/usage", s.handleUsage)
			r.Get("/usage/timeseries", s.handleUsageTimeseries)
			r.Get("/billing", s.handleBilling)

			r.Get("/custom-domains", s.handleListDomains)
			r.Post("/custom-domains", s.handleCreateDomain)
			r.Get("/custom-domains/{id}", s.handleGetDomain)
			r.Post("/custom-domains/{id}/verify", s.handleVerifyDomain)
			r.Delete("/custom-domains/{id}", s.handleDeleteDomain)

			r.Get("/audit-logs", s.handleListAudit)

			// Compliance (spec §15.6) — owner-only data deletion.
			r.Post("/data-deletion-requests", s.handleDataDeletion)
		})

		// Invite accept (uses invite token in body, not session).
		r.Post("/members/accept", s.handleAcceptInvite)

		// Data-plane (spec §8/§12) — instance_api_key OR console session (§14).
		r.Group(func(r chi.Router) {
			r.Use(s.requireData)
			r.Use(s.csrfGuard)          // session callers need CSRF; bearer callers bypass
			r.Use(s.dataWriteRateLimit) // per-tenant write throttle (spec §19.6)
			r.Post("/preview-links", s.handleCreateLink)
			r.Get("/preview-links", s.handleListLinks)
			r.Post("/preview-links/{id}/revoke", s.handleRevokeLink)
			r.Post("/files", s.handleUploadFile)
			r.Post("/uploads/presign", s.handlePresign)
			r.Post("/uploads/{fileId}/complete", s.handleCompleteUpload)
			r.Get("/files", s.handleListFiles)
			r.Get("/files/{fileId}", s.handleGetFile)
			r.Delete("/files/{fileId}", s.handleDeleteFile)
		})
	})

	return r
}
