//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apage/apage/internal/id"
)

const testPassword = "Sup3rSecret123"

// operator is a registered console user with an active tenant.
type operator struct {
	c        *client
	userID   string
	tenantID string
	email    string
}

// register creates a fresh operator (unique email) and returns an authenticated
// client carrying the session + CSRF cookies and the active tenant.
func (s *stack) register() *operator {
	s.t.Helper()
	c := s.newClient()
	email := fmt.Sprintf("e2e+%d@example.com", time.Now().UnixNano())
	r := c.req(http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email": email, "password": testPassword, "tenantName": "E2E Co",
	})
	if r.status != http.StatusCreated {
		s.t.Fatalf("register: status %d body %s", r.status, r.body)
	}
	j := r.json()
	tenantID, _ := j["tenantId"].(string)
	userID, _ := j["userId"].(string)
	if tenantID == "" || userID == "" {
		s.t.Fatalf("register: missing ids in %s", r.body)
	}
	c.tenantID = tenantID
	if c.csrf == "" {
		s.t.Fatalf("register: no CSRF cookie issued (cookies=%v)", c.cookies)
	}
	return &operator{c: c, userID: userID, tenantID: tenantID, email: email}
}

// createInstance provisions a cloud instance and returns its id + instance api key.
// In cloud-only mode there is no agent token.
func (op *operator) createInstance(t *testing.T, subdomain string) (instanceID, apiKey string) {
	t.Helper()
	r := op.c.req(http.MethodPost, "/api/v1/instances", map[string]any{
		"agentType": "custom", "subdomain": subdomain, "mode": "cloud",
	})
	if r.status != http.StatusCreated {
		t.Fatalf("create instance: status %d body %s", r.status, r.body)
	}
	j := r.json()
	instanceID, _ = j["instanceId"].(string)
	apiKey, _ = j["instanceApiKey"].(string)
	if _, hasToken := j["agentToken"]; hasToken {
		t.Errorf("create instance: cloud-only mode must not return an agentToken: %s", r.body)
	}
	if instanceID == "" || apiKey == "" {
		t.Fatalf("create instance: missing credentials in %s", r.body)
	}
	return instanceID, apiKey
}

// keyClient returns a client authenticated as the agent (instance api key).
func (s *stack) keyClient(apiKey string) *client {
	kc := s.newClient()
	kc.bearer = apiKey
	return kc
}

// uploadReady uploads content via the agent key, then waits for the worker scan
// to mark it ready, returning the cloud file id.
func (s *stack) uploadReady(kc *client, name, mime string, content []byte) string {
	s.t.Helper()
	up := kc.uploadFile(map[string]string{"displayName": name, "expiresInSeconds": "3600"}, name, mime, content)
	if up.status != http.StatusOK {
		s.t.Fatalf("upload %s: status %d body %s", name, up.status, up.body)
	}
	fileID, _ := up.json()["fileId"].(string)
	if fileID == "" {
		s.t.Fatalf("upload %s: no fileId in %s", name, up.body)
	}
	if got := waitFileStatus(s.t, kc, fileID, "ready", 20*time.Second); got != "ready" {
		s.t.Fatalf("file %s never became ready (last status %q)", name, got)
	}
	return fileID
}

// createCloudLink creates a cloud preview link via the agent key and returns its
// (linkId, secret). extra merges into the request body (displayName, password,
// accessPolicy, expiresInSeconds).
func (s *stack) createCloudLink(kc *client, fileID string, extra map[string]any) (linkID, secret string) {
	s.t.Helper()
	body := map[string]any{"mode": "cloud", "fileId": fileID}
	for k, v := range extra {
		body[k] = v
	}
	lr := kc.req(http.MethodPost, "/api/v1/preview-links", body)
	if lr.status != http.StatusCreated {
		s.t.Fatalf("create cloud link: status %d body %s", lr.status, lr.body)
	}
	return linkParts(s.t, lr.json()["url"].(string))
}

// uniqueSub returns a short, valid, unique-ish subdomain.
func uniqueSub(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano()%100000)
}

// unlockCookieName mirrors the api package's per-link unlock cookie name.
func unlockCookieName(linkID string) string { return "apage_unlock_" + linkID }

// ============================ Health =====================================

func TestHealthEndpoints(t *testing.T) {
	s := newStack(t)
	c := s.newClient()
	for _, path := range []string{"/healthz", "/readyz"} {
		r := c.req(http.MethodGet, path, nil)
		if r.status != http.StatusOK {
			t.Errorf("api %s: status %d body %s", path, r.status, r.body)
		}
	}
}

// ============================ Auth + CSRF =================================

func TestAuthAndCSRF(t *testing.T) {
	s := newStack(t)
	op := s.register()

	r := op.c.req(http.MethodGet, "/api/v1/auth/session", nil)
	if r.status != http.StatusOK {
		t.Fatalf("session: status %d body %s", r.status, r.body)
	}

	// CSRF enforcement: a state-changing session request WITHOUT the header is 403.
	bad := *op.c
	bad.csrf = ""
	br := bad.req(http.MethodPost, "/api/v1/instances", map[string]any{
		"agentType": "custom", "subdomain": uniqueSub("csrf"), "mode": "cloud",
	})
	if br.status != http.StatusForbidden {
		t.Errorf("missing CSRF token: want 403, got %d body %s", br.status, br.body)
	}

	// Wrong-password login is rejected (anti-enumeration => not 200).
	lr := s.newClient().req(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": op.email, "password": "wrong-password-9",
	})
	if lr.status == http.StatusOK {
		t.Errorf("login with wrong password unexpectedly succeeded: %s", lr.body)
	}

	// Correct login on a fresh client succeeds and yields a session.
	fresh := s.newClient()
	ok := fresh.req(http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": op.email, "password": testPassword,
	})
	if ok.status != http.StatusOK {
		t.Fatalf("login: status %d body %s", ok.status, ok.body)
	}
	if fresh.cookies["apage_session"] == "" {
		t.Errorf("login set no session cookie")
	}

	out := fresh.req(http.MethodPost, "/api/v1/auth/logout", nil)
	if out.status != http.StatusOK {
		t.Errorf("logout: status %d body %s", out.status, out.body)
	}
}

// ============================ Instance lifecycle =========================

func TestInstanceLifecycle(t *testing.T) {
	s := newStack(t)
	op := s.register()
	sub := uniqueSub("inst")
	instanceID, _ := op.createInstance(t, sub)

	lr := op.c.req(http.MethodGet, "/api/v1/instances", nil)
	if lr.status != http.StatusOK || !strings.Contains(string(lr.body), instanceID) {
		t.Errorf("list instances: status %d body %s", lr.status, lr.body)
	}

	gr := op.c.req(http.MethodGet, "/api/v1/instances/"+instanceID, nil)
	if gr.status != http.StatusOK {
		t.Errorf("get instance: status %d body %s", gr.status, gr.body)
	}

	// Subdomains are globally unique: a DIFFERENT tenant claiming the same
	// subdomain is rejected with 409.
	op2 := s.register()
	dup := op2.c.req(http.MethodPost, "/api/v1/instances", map[string]any{
		"agentType": "custom", "subdomain": sub, "mode": "cloud",
	})
	if dup.status != http.StatusConflict {
		t.Errorf("cross-tenant duplicate subdomain: want 409, got %d body %s", dup.status, dup.body)
	}

	// Reserved/invalid subdomain rejected with 400.
	bad := op2.c.req(http.MethodPost, "/api/v1/instances", map[string]any{
		"agentType": "custom", "subdomain": "admin", "mode": "cloud",
	})
	if bad.status != http.StatusBadRequest {
		t.Errorf("reserved subdomain: want 400, got %d body %s", bad.status, bad.body)
	}

	del := op.c.req(http.MethodDelete, "/api/v1/instances/"+instanceID, nil)
	if del.status != http.StatusOK {
		t.Errorf("delete instance: status %d body %s", del.status, del.body)
	}
}

// ============================ Cloud preview (full journey) ===============

func TestCloudUploadFlow(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("cloud"))
	kc := s.keyClient(apiKey)

	content := []byte("# Cloud doc\n\nstored in MinIO via APAGE e2e\n")
	up := kc.uploadFile(map[string]string{"displayName": "doc.txt", "expiresInSeconds": "3600"},
		"doc.txt", "text/plain", content)
	if up.status != http.StatusOK {
		t.Fatalf("upload: status %d body %s", up.status, up.body)
	}
	fileID, _ := up.json()["fileId"].(string)
	if fileID == "" {
		t.Fatalf("upload: no fileId in %s", up.body)
	}
	if st, _ := up.json()["status"].(string); st == "ready" {
		t.Errorf("upload: initial status must not be ready (P0-1), got %q", st)
	}
	if got := waitFileStatus(t, kc, fileID, "ready", 20*time.Second); got != "ready" {
		t.Fatalf("file never became ready (last status %q)", got)
	}

	linkID, secret := s.createCloudLink(kc, fileID, map[string]any{"displayName": "doc.txt"})

	// Full preview.
	vis := s.newClient()
	pr := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if pr.status != http.StatusOK {
		t.Fatalf("cloud preview: status %d body %s", pr.status, pr.body)
	}
	if string(pr.body) != string(content) {
		t.Errorf("cloud preview body mismatch:\n got %q\nwant %q", pr.body, content)
	}

	// Range -> 206 partial.
	rr := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, map[string]string{"Range": "bytes=0-4"})
	if rr.status != http.StatusPartialContent {
		t.Errorf("range request: want 206, got %d", rr.status)
	} else if string(rr.body) != string(content[:5]) {
		t.Errorf("range body = %q, want %q", rr.body, content[:5])
	}

	// Wrong secret -> 404 (does not leak existence).
	wr := vis.raw(http.MethodGet, "/p/"+linkID+"/"+id.NewSecret(id.SecretPreviewLink), nil)
	if wr.status != http.StatusNotFound {
		t.Errorf("wrong secret: want 404, got %d", wr.status)
	}

	// Revoke -> 410.
	rv := kc.req(http.MethodPost, "/api/v1/preview-links/"+linkID+"/revoke", nil)
	if rv.status != http.StatusOK {
		t.Fatalf("revoke: status %d body %s", rv.status, rv.body)
	}
	if gone := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); gone.status != http.StatusGone {
		t.Errorf("revoked link: want 410, got %d body %s", gone.status, gone.body)
	}

	// Delete the file.
	if del := kc.req(http.MethodDelete, "/api/v1/files/"+fileID, nil); del.status != http.StatusOK {
		t.Errorf("delete file: status %d body %s", del.status, del.body)
	}
}

// ============================ Links are agent-only =======================

// TestConsoleCannotCreateLink verifies a console session cannot create preview
// links — only the agent (instance api key) may. The role check passes (admin),
// so a 403 here proves the ViaKey gate, not RBAC.
func TestConsoleCannotCreateLink(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("acl"))
	kc := s.keyClient(apiKey)
	fileID := s.uploadReady(kc, "doc.txt", "text/plain", []byte("agent-only"))

	// Console session attempt -> 403.
	r := op.c.req(http.MethodPost, "/api/v1/preview-links", map[string]any{"mode": "cloud", "fileId": fileID})
	if r.status != http.StatusForbidden {
		t.Errorf("console create link: want 403, got %d body %s", r.status, r.body)
	}
	// Agent attempt -> 201.
	if linkID, _ := s.createCloudLink(kc, fileID, nil); linkID == "" {
		t.Errorf("agent create link produced no link id")
	}
}

// ============================ modify_link (replace content) ==============

// TestModifyLinkReplacesContent verifies the PATCH endpoint backing modify_link:
// swapping the file keeps the same URL but serves the new content.
func TestModifyLinkReplacesContent(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("mod"))
	kc := s.keyClient(apiKey)

	fileA := s.uploadReady(kc, "v1.txt", "text/plain", []byte("VERSION ONE"))
	linkID, secret := s.createCloudLink(kc, fileA, map[string]any{"displayName": "doc.txt"})

	vis := s.newClient()
	if r := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); string(r.body) != "VERSION ONE" {
		t.Fatalf("pre-modify preview = %q, want VERSION ONE", r.body)
	}

	fileB := s.uploadReady(kc, "v2.txt", "text/plain", []byte("VERSION TWO"))
	pr := kc.req(http.MethodPatch, "/api/v1/preview-links/"+linkID, map[string]any{"fileId": fileB})
	if pr.status != http.StatusOK {
		t.Fatalf("modify link: status %d body %s", pr.status, pr.body)
	}

	// Same URL now serves the new content (cache invalidated on update).
	got := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if got.status != http.StatusOK || string(got.body) != "VERSION TWO" {
		t.Errorf("post-modify preview: status %d body %q, want VERSION TWO", got.status, got.body)
	}

	// A console session cannot modify links.
	cr := op.c.req(http.MethodPatch, "/api/v1/preview-links/"+linkID, map[string]any{"displayName": "x"})
	if cr.status != http.StatusForbidden {
		t.Errorf("console modify link: want 403, got %d body %s", cr.status, cr.body)
	}
}

// ============================ Access policies (cloud) ====================

func TestPasswordPolicy(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("pw"))
	kc := s.keyClient(apiKey)
	content := []byte("secret content")
	fileID := s.uploadReady(kc, "secret.txt", "text/plain", content)
	linkID, secret := s.createCloudLink(kc, fileID, map[string]any{"password": "open-sesame-7"})

	vis := s.newClient()
	gate := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if gate.status != http.StatusOK || !strings.Contains(string(gate.body), "password protected") {
		t.Fatalf("password gate: status %d body %s", gate.status, gate.body)
	}
	if strings.Contains(string(gate.body), "secret content") {
		t.Errorf("password gate leaked content")
	}

	wrong := vis.req(http.MethodPost, "/p/"+linkID+"/"+secret+"/unlock", map[string]any{"password": "nope"})
	if wrong.status != http.StatusForbidden {
		t.Errorf("wrong password: want 403, got %d body %s", wrong.status, wrong.body)
	}

	right := vis.req(http.MethodPost, "/p/"+linkID+"/"+secret+"/unlock", map[string]any{"password": "open-sesame-7"})
	if right.status != http.StatusOK {
		t.Fatalf("correct password: status %d body %s", right.status, right.body)
	}
	if vis.cookies[unlockCookieName(linkID)] == "" {
		t.Fatalf("unlock set no cookie (cookies=%v)", vis.cookies)
	}

	open := vis.raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if open.status != http.StatusOK || string(open.body) != string(content) {
		t.Errorf("unlocked preview: status %d body %q", open.status, open.body)
	}
}

func TestMaxViewsPolicy(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("mv"))
	kc := s.keyClient(apiKey)
	fileID := s.uploadReady(kc, "limited.txt", "text/plain", []byte("limited views"))
	linkID, secret := s.createCloudLink(kc, fileID, map[string]any{
		"accessPolicy": map[string]any{"type": "public", "maxViews": 2},
	})

	// 4 concurrent requests against maxViews=2 => exactly 2x200 / 2x410 (atomic).
	const n = 4
	var ok200, gone410 int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
			switch r.status {
			case http.StatusOK:
				atomic.AddInt64(&ok200, 1)
			case http.StatusGone:
				atomic.AddInt64(&gone410, 1)
			default:
				t.Errorf("maxViews request: unexpected status %d body %s", r.status, r.body)
			}
		}()
	}
	wg.Wait()
	if ok200 != 2 || gone410 != 2 {
		t.Errorf("maxViews=2 under %d concurrent: got %d x200 / %d x410, want 2/2", n, ok200, gone410)
	}
}

func TestSingleUsePolicy(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("su"))
	kc := s.keyClient(apiKey)
	content := []byte("one and done")
	fileID := s.uploadReady(kc, "once.txt", "text/plain", content)
	linkID, secret := s.createCloudLink(kc, fileID, map[string]any{
		"accessPolicy": map[string]any{"type": "public", "singleUse": true},
	})

	first := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if first.status != http.StatusOK {
		t.Fatalf("single-use first view: want 200, got %d body %s", first.status, first.body)
	}
	second := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if second.status != http.StatusGone {
		t.Errorf("single-use second view: want 410, got %d body %s", second.status, second.body)
	}
}

func TestIPAllowlistPolicy(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("ip"))
	kc := s.keyClient(apiKey)
	fileID := s.uploadReady(kc, "gated.txt", "text/plain", []byte("ip gated"))
	// Allowlist excludes the visitor's TEST-NET IP -> denied.
	linkID, secret := s.createCloudLink(kc, fileID, map[string]any{
		"accessPolicy": map[string]any{"type": "ip_allowlist", "ipAllowlist": []string{"10.0.0.0/8"}},
	})

	denied := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil)
	if denied.status != http.StatusForbidden {
		t.Errorf("ip-allowlist deny: want 403, got %d body %s", denied.status, denied.body)
	}
}

// ============================ Freeze (abuse hold) =======================

func TestFrozenLinkBlocksPreview(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("frz"))
	kc := s.keyClient(apiKey)
	content := []byte("freezable")
	fileID := s.uploadReady(kc, "f.txt", "text/plain", content)
	linkID, secret := s.createCloudLink(kc, fileID, nil)

	if r := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); r.status != http.StatusOK {
		t.Fatalf("pre-freeze preview: want 200, got %d body %s", r.status, r.body)
	}

	// Owner freezes the link (abuse hold) — session path, admin role required.
	fr := op.c.req(http.MethodPost, "/api/v1/preview-links/"+linkID+"/freeze", map[string]any{"reason": "e2e abuse test"})
	if fr.status != http.StatusOK {
		t.Fatalf("freeze link: status %d body %s", fr.status, fr.body)
	}

	if gone := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); gone.status != http.StatusGone {
		t.Errorf("frozen link: want 410, got %d body %s", gone.status, gone.body)
	}

	if uf := op.c.req(http.MethodPost, "/api/v1/preview-links/"+linkID+"/unfreeze", nil); uf.status != http.StatusOK {
		t.Fatalf("unfreeze: status %d body %s", uf.status, uf.body)
	}
	if r := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); r.status != http.StatusOK {
		t.Errorf("post-unfreeze preview: want 200, got %d body %s", r.status, r.body)
	}
}

// ============================ Cross-surface audit trail =================

func TestAuditTrailRecorded(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("aud"))
	kc := s.keyClient(apiKey)
	fileID := s.uploadReady(kc, "a.txt", "text/plain", []byte("audited content"))
	linkID, secret := s.createCloudLink(kc, fileID, nil)

	if r := s.newClient().raw(http.MethodGet, "/p/"+linkID+"/"+secret, nil); r.status != http.StatusOK {
		t.Fatalf("preview: status %d body %s", r.status, r.body)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ar := op.c.req(http.MethodGet, "/api/v1/audit-logs?resourceId="+linkID+"&event=preview_link.accessed", nil)
		if ar.status == http.StatusOK && strings.Contains(string(ar.body), "preview_link.accessed") &&
			strings.Contains(string(ar.body), linkID) {
			return // success
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("preview_link.accessed audit entry for %s never appeared", linkID)
}

// ============================ MIME allowlist ============================

func TestUploadRejectsDisallowedMime(t *testing.T) {
	s := newStack(t)
	op := s.register()
	_, apiKey := op.createInstance(t, uniqueSub("mime"))
	kc := s.keyClient(apiKey)

	up := kc.uploadFile(map[string]string{"displayName": "evil.exe"},
		"evil.exe", "application/x-msdownload", []byte("MZ\x90\x00binary"))
	if up.status != http.StatusUnsupportedMediaType {
		t.Errorf("disallowed mime upload: want 415, got %d body %s", up.status, up.body)
	}
}

// waitFileStatus polls GET /files/{id} until status==want or timeout.
func waitFileStatus(t *testing.T, c *client, fileID, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		r := c.req(http.MethodGet, "/api/v1/files/"+fileID, nil)
		if r.status == http.StatusOK {
			last, _ = r.json()["status"].(string)
			if last == want || last == "rejected" {
				return last
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return last
}
