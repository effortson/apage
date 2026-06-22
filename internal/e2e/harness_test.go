//go:build e2e

// Package e2e contains in-process, multi-surface end-to-end tests for APAGE.
//
// Unlike the unit tests, these wire the REAL servers — apage-api and the worker —
// together against the live infra (Postgres :5433, Redis :6379, MinIO :9100, as
// started by `make infra-up`). They simulate the full operator + agent (instance
// API key) + visitor journey for the cloud-only flow and assert on the observable
// behavior of every surface.
//
// Run with:
//
//	make infra-up           # or: docker compose up -d --wait postgres redis minio
//	go test -tags e2e ./internal/e2e/...
//
// The harness skips (does not fail) when the infra is unreachable so it never
// breaks a no-infra `go test ./...`.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apage/apage/internal/api"
	"github.com/apage/apage/internal/config"
	"github.com/apage/apage/internal/mail"
	"github.com/apage/apage/internal/objstore"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
	"github.com/apage/apage/internal/worker"
)

// stack holds a fully wired in-process APAGE deployment for one test.
type stack struct {
	t   *testing.T
	cfg *config.Config
	db  *store.Store
	rdb *redisx.Client
	obj *objstore.Store

	apiURL string

	apiServer *httptest.Server

	workerCancel context.CancelFunc
}

// newStack brings up api + worker against the live infra. It calls t.Skip (not
// t.Fatal) when the datastores are unreachable.
func newStack(t *testing.T) *stack {
	t.Helper()

	// Match docker-compose host mappings (see scripts/dev.sh).
	env := map[string]string{
		"APP_ENV":            "development",
		"DATABASE_URL":       firstNonEmpty(os.Getenv("DATABASE_URL"), "postgres://apage:apage@localhost:5433/apage?sslmode=disable"),
		"REDIS_URL":          firstNonEmpty(os.Getenv("REDIS_URL"), "redis://localhost:6379/0"),
		"S3_ENDPOINT":        firstNonEmpty(os.Getenv("S3_ENDPOINT"), "http://localhost:9100"),
		"S3_PUBLIC_ENDPOINT": "", // empty => serveCloud streams bytes (no redirect) for deterministic assertions
		"S3_BUCKET":          firstNonEmpty(os.Getenv("S3_BUCKET"), "apage"),
		"S3_ACCESS_KEY":      firstNonEmpty(os.Getenv("S3_ACCESS_KEY"), "minioadmin"),
		"S3_SECRET_KEY":      firstNonEmpty(os.Getenv("S3_SECRET_KEY"), "minioadmin"),
		"APP_BASE_DOMAIN":    "preview.localhost",
		"SESSION_SECRET":     "e2e-session-secret",
		"JWT_SIGNING_SECRET": "e2e-jwt-secret",
		// TrustedProxyCount 1 => ClientIP is the single X-Forwarded-For value each
		// client sends. This gives every client its own stable, unique IP so that
		// per-IP rate limits (register 10/h, login, preview, unlock) are isolated
		// per client and the suite stays deterministic and re-runnable.
		"TRUSTED_PROXY_COUNT": "1",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	ctx := context.Background()
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Skipf("postgres unreachable (%v); start infra with `make infra-up`", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rdb, err := redisx.New(cfg.RedisURL)
	if err != nil {
		db.Close()
		t.Skipf("redis unreachable (%v); start infra with `make infra-up`", err)
	}

	obj, err := objstore.New(objstore.Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, Bucket: cfg.S3Bucket,
		Region: cfg.S3Region, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL,
		PresignTTL: time.Duration(cfg.PresignURLTTLSeconds) * time.Second,
	})
	if err != nil {
		rdb.Close()
		db.Close()
		t.Skipf("minio unreachable (%v); start infra with `make infra-up`", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	apiSrv := api.New(cfg, db, rdb, log, mail.LogMailer{Log: log}, obj)
	apiServer := httptest.NewServer(apiSrv.Router())

	// Worker drains the scan/delete/audit queues for the cloud path.
	wctx, wcancel := context.WithCancel(context.Background())
	w := worker.New(db, rdb, obj, log, 0) // auditRetentionDays 0 => no purge
	go w.Run(wctx)

	s := &stack{
		t: t, cfg: cfg, db: db, rdb: rdb, obj: obj,
		apiURL: apiServer.URL, apiServer: apiServer, workerCancel: wcancel,
	}
	t.Cleanup(s.close)
	return s
}

func (s *stack) close() {
	s.workerCancel()
	s.apiServer.Close()
	s.rdb.Close()
	s.db.Close()
}

// --- HTTP client (manual cookie management) --------------------------------

// client is a minimal HTTP client that tracks cookies manually. The session and
// CSRF cookies are flagged Secure, which Go's cookiejar refuses to send over the
// httptest http:// transport — so we attach them ourselves.
type client struct {
	t        *testing.T
	base     string
	http     *http.Client
	cookies  map[string]string
	csrf     string
	tenantID string
	bearer   string // instance api key; when set, used instead of session
	clientIP string // sent as X-Forwarded-For so each client has a distinct IP
}

func (s *stack) newClient() *client {
	return &client{
		t: s.t, base: s.apiURL, cookies: map[string]string{}, clientIP: nextClientIP(),
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Do not auto-follow redirects: tests assert on 3xx explicitly.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// ipSeq + ipSeedOnce hand out unique client IPs from the TEST-NET-3 (203.0.113/
// 203.0/16) space. The base is randomized per process so repeated `go test`
// runs within the rate-limit window don't collide on the same IPs.
var (
	ipSeq      atomic.Uint32
	ipSeedOnce sync.Once
)

func nextClientIP() string {
	ipSeedOnce.Do(func() { ipSeq.Store(uint32(time.Now().UnixNano())) })
	n := ipSeq.Add(1)
	return fmt.Sprintf("203.0.%d.%d", (n>>8)&0xFF, n&0xFF)
}

// resp is a decoded HTTP response.
type resp struct {
	status  int
	body    []byte
	headers http.Header
}

func (r *resp) json() map[string]any {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	return m
}

func (c *client) req(method, path string, body any) *resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.decorate(req, method)
	return c.send(req)
}

// decorate attaches auth: a bearer key if set, else session + CSRF cookies.
func (c *client) decorate(req *http.Request, method string) {
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
		return
	}
	for name, val := range c.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: val})
	}
	if c.tenantID != "" {
		req.Header.Set("X-Tenant-Id", c.tenantID)
	}
	if c.csrf != "" && !safeMethod(method) {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
}

func (c *client) send(req *http.Request) *resp {
	c.t.Helper()
	if c.clientIP != "" && req.Header.Get("X-Forwarded-For") == "" {
		req.Header.Set("X-Forwarded-For", c.clientIP)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	// Capture any Set-Cookie updates (session, csrf, unlock).
	for _, ck := range res.Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
			continue
		}
		c.cookies[ck.Name] = ck.Value
		if ck.Name == "apage_csrf" {
			c.csrf = ck.Value
		}
	}
	return &resp{status: res.StatusCode, body: b, headers: res.Header}
}

// raw issues a request to an absolute-or-relative runtime path (e.g. /p/...)
// with optional extra headers, returning the response without auth decoration.
func (c *client) raw(method, path string, headers map[string]string) *resp {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, nil)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Attach any unlock cookies relevant to /p/{linkId} paths.
	for name, val := range c.cookies {
		if strings.HasPrefix(name, "apage_unlock_") {
			req.AddCookie(&http.Cookie{Name: name, Value: val})
		}
	}
	return c.send(req)
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// multipartUpload builds a multipart body for the direct-upload endpoint. The
// file part carries the given fileContentType, exactly as a browser/agent would
// set it — the API reads the declared MIME from this part header (files.go).
func multipartUpload(fields map[string]string, fileField, fileName, fileContentType string, content []byte) (body *bytes.Buffer, contentType string) {
	body = &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, fileField, fileName))
	if fileContentType != "" {
		h.Set("Content-Type", fileContentType)
	}
	fw, _ := mw.CreatePart(h)
	_, _ = fw.Write(content)
	_ = mw.Close()
	return body, mw.FormDataContentType()
}

// uploadFile performs a multipart POST /api/v1/files using the client's auth.
func (c *client) uploadFile(fields map[string]string, fileName, fileContentType string, content []byte) *resp {
	c.t.Helper()
	body, ct := multipartUpload(fields, "file", fileName, fileContentType, content)
	req, err := http.NewRequest(http.MethodPost, c.base+"/api/v1/files", body)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", ct)
	c.decorate(req, http.MethodPost)
	return c.send(req)
}

// linkParts extracts (linkId, secret) from a preview URL of the form
// https://<sub>.<domain>/p/<linkId>/<secret>.
func linkParts(t *testing.T, rawURL string) (linkID, secret string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse link url %q: %v", rawURL, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "p" {
		t.Fatalf("unexpected link path %q", u.Path)
	}
	return parts[1], parts[2]
}
