// Package config loads runtime configuration from environment variables.
// Spec §33: all configuration is injected via env vars; nothing is hardcoded.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the resolved runtime configuration shared across services.
type Config struct {
	// Environment is "production" or "development" (APP_ENV). In production,
	// Validate() refuses to boot with insecure default secrets.
	Environment string

	// Domains (spec §5, §33)
	BaseDomain    string // APP_BASE_DOMAIN, e.g. preview.example.com
	ConsoleDomain string // APP_CONSOLE_DOMAIN
	RenderDomain  string // APP_RENDER_DOMAIN
	// CookieDomain, when set (e.g. ".example.com"), scopes the session/CSRF
	// cookies to the parent domain so account-gated previews on preview
	// subdomains can read the console session (spec §14/§25). Empty => host-only.
	CookieDomain string

	// TrustedProxyCount is the number of trusted reverse proxies in front of the
	// API. The real client IP is taken that many hops from the right of
	// X-Forwarded-For, so a client-spoofed leftmost value cannot be trusted
	// (spec §14). 0 => do not trust X-Forwarded-For at all (use RemoteAddr).
	TrustedProxyCount int

	// Datastores
	DatabaseURL string // DATABASE_URL
	RedisURL    string // REDIS_URL

	// Object storage (spec §11)
	S3Endpoint string
	// S3PublicEndpoint is the browser-reachable host used to presign GET/PUT URLs
	// (the main endpoint may be internal-only, e.g. docker `minio:9000`). When set,
	// the API may redirect cloud previews to signed URLs instead of proxying (§19.3).
	S3PublicEndpoint string
	S3Bucket         string
	S3Region         string
	S3AccessKey      string
	S3SecretKey      string
	S3UseSSL         bool
	// S3LifecycleDays sets a bucket lifecycle rule expiring all objects after N
	// days as an orphan-cleanup backstop (0 = disabled). Set well beyond the
	// longest plan retention so it never deletes live files (spec §19.3).
	S3LifecycleDays int

	// Secrets
	JWTSigningSecret string
	SessionSecret    string

	// Agent compatibility (spec §6.1, §7)
	AgentMinProtocolVersion string
	AgentMinVersion         string

	// Upload thresholds (spec §12)
	DirectUploadMaxBytes int64
	PresignURLTTLSeconds int

	// Mail (spec §25)
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	MailFrom string

	// Abuse governance (spec §15.5, V1)
	SafeBrowsingAPIKey string

	// AuditRetentionDays: audit logs older than this are purged (spec §11/§15.6).
	AuditRetentionDays int

	// Admin console (spec §8). Empty allowlist => no IP restriction (dev only).
	AdminIPAllowlist       []string // comma-separated CIDRs/IPs
	AdminBootstrapEmail    string   // seeds the first platform admin if the table is empty
	AdminBootstrapPassword string

	// OAuth providers (spec §25). Empty client id/secret => provider disabled.
	OAuthRedirectBase   string // base URL for the callback redirect_uri (default https://<ConsoleDomain>)
	OAuthGitHubClientID string
	OAuthGitHubSecret   string
	OAuthGoogleClientID string
	OAuthGoogleSecret   string

	// Service bind addresses
	APIAddr     string // :8080
	GatewayAddr string // :8090
	MetricsAddr string // :9090

	// Internal URL the API uses to reach the gateway for tunnel streaming
	// (fallback when the registry has no per-instance gateway URL).
	GatewayInternalURL string
	// GatewayInternalSecret authenticates the API -> gateway internal stream
	// endpoint so it cannot be driven directly even if the agent host exposes it
	// (spec §19.4). Shared by the API and gateway; required in production.
	GatewayInternalSecret string
	// GatewayAdvertiseURL is the URL this gateway publishes to the registry so the
	// API can route previews to it (multi-gateway). Defaults to GatewayInternalURL.
	GatewayAdvertiseURL string

	// Gateway session defaults (spec §7)
	MaxConcurrentStreams int
	MaxChunkBytes        int
	IdleTimeoutSeconds   int
}

// Load reads configuration from the environment, applying defaults where safe.
func Load() (*Config, error) {
	c := &Config{
		Environment:             strings.ToLower(env("APP_ENV", "development")),
		BaseDomain:              env("APP_BASE_DOMAIN", "preview.localhost"),
		ConsoleDomain:           env("APP_CONSOLE_DOMAIN", "console.localhost"),
		RenderDomain:            env("APP_RENDER_DOMAIN", "render.preview.localhost"),
		CookieDomain:            env("COOKIE_DOMAIN", ""),
		TrustedProxyCount:       envInt("TRUSTED_PROXY_COUNT", 1),
		DatabaseURL:             env("DATABASE_URL", "postgres://apage:apage@localhost:5432/apage?sslmode=disable"),
		RedisURL:                env("REDIS_URL", "redis://localhost:6379/0"),
		S3Endpoint:              env("S3_ENDPOINT", "http://localhost:9000"),
		S3PublicEndpoint:        env("S3_PUBLIC_ENDPOINT", ""),
		S3Bucket:                env("S3_BUCKET", "apage"),
		S3Region:                env("S3_REGION", "us-east-1"),
		S3AccessKey:             env("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:             env("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:                envBool("S3_USE_SSL", false),
		S3LifecycleDays:         envInt("S3_LIFECYCLE_DAYS", 0),
		JWTSigningSecret:        env("JWT_SIGNING_SECRET", "dev-jwt-secret-change-me"),
		SessionSecret:           env("SESSION_SECRET", "dev-session-secret-change-me"),
		AgentMinProtocolVersion: env("AGENT_MIN_PROTOCOL_VERSION", "1"),
		AgentMinVersion:         env("AGENT_MIN_VERSION", "0.1.0"),
		DirectUploadMaxBytes:    envInt64("DIRECT_UPLOAD_MAX_BYTES", 8*1024*1024),
		PresignURLTTLSeconds:    envInt("PRESIGN_URL_TTL_SECONDS", 900),
		SMTPHost:                env("SMTP_HOST", ""),
		SMTPPort:                envInt("SMTP_PORT", 587),
		SMTPUser:                env("SMTP_USER", ""),
		SMTPPass:                env("SMTP_PASS", ""),
		MailFrom:                env("MAIL_FROM", "no-reply@apage.local"),
		SafeBrowsingAPIKey:      env("SAFE_BROWSING_API_KEY", ""),
		AuditRetentionDays:      envInt("AUDIT_RETENTION_DAYS", 90),
		AdminIPAllowlist:        envList("ADMIN_IP_ALLOWLIST"),
		AdminBootstrapEmail:     env("ADMIN_BOOTSTRAP_EMAIL", ""),
		AdminBootstrapPassword:  env("ADMIN_BOOTSTRAP_PASSWORD", ""),
		OAuthRedirectBase:       env("OAUTH_REDIRECT_BASE", ""),
		OAuthGitHubClientID:     env("OAUTH_GITHUB_CLIENT_ID", ""),
		OAuthGitHubSecret:       env("OAUTH_GITHUB_CLIENT_SECRET", ""),
		OAuthGoogleClientID:     env("OAUTH_GOOGLE_CLIENT_ID", ""),
		OAuthGoogleSecret:       env("OAUTH_GOOGLE_CLIENT_SECRET", ""),
		APIAddr:                 env("API_ADDR", ":8080"),
		GatewayAddr:             env("GATEWAY_ADDR", ":8090"),
		MetricsAddr:             env("METRICS_ADDR", ":9090"),
		GatewayInternalURL:      env("GATEWAY_INTERNAL_URL", "http://localhost:8090"),
		GatewayInternalSecret:   env("GATEWAY_INTERNAL_SECRET", ""),
		GatewayAdvertiseURL:     env("GATEWAY_ADVERTISE_URL", ""),
		MaxConcurrentStreams:    envInt("MAX_CONCURRENT_STREAMS", 16),
		MaxChunkBytes:           envInt("MAX_CHUNK_BYTES", 262144),
		IdleTimeoutSeconds:      envInt("IDLE_TIMEOUT_SECONDS", 60),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.GatewayAdvertiseURL == "" {
		c.GatewayAdvertiseURL = c.GatewayInternalURL
	}
	if c.OAuthRedirectBase == "" {
		c.OAuthRedirectBase = "https://" + c.ConsoleDomain
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// IsProduction reports whether the service is running in a production-like
// environment, where insecure default secrets must be rejected.
func (c *Config) IsProduction() bool {
	switch c.Environment {
	case "", "dev", "development", "test", "local":
		return false
	}
	return true
}

// insecureDefaults are the dev placeholder secrets that must never reach prod.
// Empty also counts: these values are always required in production.
var insecureDefaults = map[string]bool{
	"dev-jwt-secret-change-me":      true,
	"dev-session-secret-change-me":  true,
	"dev-internal-secret-change-me": true,
	"":                              true,
}

// Validate refuses to boot in production with default/empty secrets so a
// misconfigured deploy cannot ship forgeable session/grant tokens or an
// unauthenticated gateway stream endpoint (security review #4/#3).
func (c *Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	var bad []string
	if insecureDefaults[c.SessionSecret] {
		bad = append(bad, "SESSION_SECRET")
	}
	if insecureDefaults[c.JWTSigningSecret] {
		bad = append(bad, "JWT_SIGNING_SECRET")
	}
	if insecureDefaults[c.GatewayInternalSecret] {
		bad = append(bad, "GATEWAY_INTERNAL_SECRET")
	}
	// S3 is optional (tunnel-only deploys need no cloud storage), so only the
	// known-dangerous "minioadmin" default is rejected — empty is allowed.
	if c.S3AccessKey == "minioadmin" || c.S3SecretKey == "minioadmin" {
		bad = append(bad, "S3_ACCESS_KEY/S3_SECRET_KEY")
	}
	if len(bad) > 0 {
		return fmt.Errorf("APP_ENV=%s but insecure/default values for: %s "+
			"(set strong secrets before running in production)", c.Environment, strings.Join(bad, ", "))
	}
	return nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// envList parses a comma-separated env var into a trimmed, non-empty slice.
func envList(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}
