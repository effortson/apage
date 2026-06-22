package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// handlePreview serves a preview link to a visitor (spec §30). No Bearer auth;
// admission is gated by the path secret + access_policy + three-layer expiry.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	secret := chi.URLParam(r, "secret")

	link, ok := s.admitLink(w, r, linkID, secret)
	if !ok {
		return
	}
	pol := parsePolicy(link.AccessPolicy)
	mime, displayName, backingExpiry, serveErr := s.resolveBacking(r, link)
	if serveErr != nil {
		s.serveError(w, r, serveErr)
		return
	}
	category := previewCategory(mime)

	// High-risk active content (HTML/SVG) is rendered safely inside a sandboxed
	// wrapper on the isolated render origin (spec §13/§15.5), handled separately.
	if isActiveContent(category) {
		s.serveActive(w, r, link, pol, category, displayName, backingExpiry)
		return
	}

	// Passive content (pdf/image/text): gates -> view consume -> inline stream.
	if !s.gateIP(w, r, link, pol) || !s.gateAccount(w, r, link, pol) {
		return
	}
	if passwordRequired(pol) && !s.hasUnlock(r, linkID) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		setSecurityHeaders(w, "text")
		_, _ = w.Write([]byte(passwordPageHTML(linkID, secret, false)))
		return
	}
	if s.backingExpired(link, backingExpiry) {
		httpx.Gone(w, r, "link expired")
		return
	}
	if !s.consumeView(w, r, link, pol) {
		return
	}
	go s.access(linkID, link.TenantID, link.InstanceID, r)
	setSecurityHeaders(w, category)
	// Redirect to a signed URL only when there is no per-view cap to enforce.
	s.serveBytes(w, r, link, displayName, pol.AllowDownload, false, maxViewsCap(pol) == 0)
}

// isActiveContent reports whether a category can execute script and so must be
// origin-isolated + sandboxed rather than rendered inline (spec §13/§15.5).
func isActiveContent(category string) bool { return category == "html" || category == "svg" }

// --- shared admission steps (reused by the passive and active paths) ---

func (s *Server) gateIP(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, pol store.AccessPolicy) bool {
	if !ipAllowed(pol, httpx.ClientIP(r.Context())) {
		s.deny(r, link, "ip_not_allowed")
		httpx.Forbidden(w, r, "access denied")
		return false
	}
	return true
}

func (s *Server) gateAccount(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, pol store.AccessPolicy) bool {
	if accountRequired(pol) && !s.accountAllowed(r, pol) {
		s.deny(r, link, "account_required")
		httpx.Err(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "login required to view this link", false)
		return false
	}
	return true
}

func (s *Server) backingExpired(link *store.PreviewLink, backingExpiry *time.Time) bool {
	eff := effectiveExpiry(link.ExpiresAt, backingExpiry)
	return eff != nil && eff.Before(time.Now())
}

// consumeView applies the strong-consistency single_use/maxViews gate (spec §14
// / P0-2). Returns false (and writes 410) when the cap is exceeded.
func (s *Server) consumeView(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, pol store.AccessPolicy) bool {
	cap := maxViewsCap(pol)
	if cap == 0 && !pol.SingleUse {
		return true
	}
	var ttl time.Duration
	if link.ExpiresAt != nil {
		ttl = time.Until(*link.ExpiresAt)
	}
	n, err := s.rdb.ConsumeView(r.Context(), link.LinkID, cap, ttl)
	if errors.Is(err, redisx.ErrQuotaExceeded) {
		s.deny(r, link, "max_views_exceeded")
		httpx.Gone(w, r, "link view limit reached")
		return false
	}
	if err == nil {
		go s.flushViewCount(link.LinkID, n)
	}
	return true
}

// serveBytes streams a link's backing bytes (tunnel via gateway or cloud object),
// honoring Range, and meters the egress it produces (spec §29). forceAttachment
// serves a download rather than inline content.
func (s *Server) serveBytes(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, name string, allowDownload, forceAttachment, allowRedirect bool) {
	cw := &countingWriter{ResponseWriter: w}
	switch link.Mode {
	case "tunnel":
		s.setDownloadHeaders(cw, name, allowDownload, forceAttachment)
		if err := s.gw.StreamFile(cw, r, link.InstanceID, *link.FileRef); err != nil {
			s.log.Warn("tunnel stream", "err", err, "instance", link.InstanceID)
			if !headersSent(cw) {
				httpx.Err(cw, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "agent offline", true)
			}
		}
	case "cloud":
		s.serveCloud(cw, r, link, name, allowDownload, forceAttachment, allowRedirect)
	}
	if cw.n > 0 {
		s.meterEgress(link.TenantID, link.Mode, cw.n)
	}
}

// countingWriter tallies bytes written to a response so egress can be metered.
type countingWriter struct {
	http.ResponseWriter
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.n += int64(n)
	return n, err
}

func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// meterEgress buffers served bytes against the tenant's egress quota (spec §29).
func (s *Server) meterEgress(tenantID, mode string, n int64) {
	dim := "tunnel_egress"
	if mode == "cloud" {
		dim = "cloud_egress"
	}
	_ = s.rdb.AddUsage(context.Background(), tenantID, dim, n)
}

// handleFileDirect serves a cloud file direct link /f/{fileId}/{secret} (spec §16/§30).
func (s *Server) handleFileDirect(w http.ResponseWriter, r *http.Request) {
	// For MVP direct links share the preview-link mechanism keyed by file.
	// A production build issues a dedicated afs_ secret per file; here we 404 to
	// avoid implying unguarded file access.
	httpx.NotFound(w, r)
}

// handleUnlock verifies a link password and sets a scoped unlock cookie (spec §30).
func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	secret := chi.URLParam(r, "secret")
	// Rate-limit password attempts (spec §14).
	if !s.limit(w, r, "unlock:"+linkID+":"+httpx.ClientIP(r.Context()), 5, 15*time.Minute) {
		return
	}
	link, ok := s.admitLink(w, r, linkID, secret)
	if !ok {
		return
	}
	pol := parsePolicy(link.AccessPolicy)
	if !passwordRequired(pol) || pol.Password == nil {
		httpx.BadRequest(w, r, "link is not password protected")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	ok2, _ := hash.VerifyPassword(body.Password, pol.Password.Hash)
	if !ok2 {
		// Do not reveal whether the link exists vs wrong password (spec §30).
		httpx.Err(w, r, http.StatusForbidden, httpx.CodeAccessDenied, "incorrect password or link unavailable", false)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: unlockCookie(linkID), Value: s.unlockToken(linkID), Path: "/p/" + linkID,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 3600,
	})
	httpx.JSON(w, http.StatusOK, map[string]bool{"unlocked": true})
}

// admitLink loads a link and runs the secret/expiry/revoked/frozen gates common
// to all runtime entrypoints (spec §30 steps 1–3).
func (s *Server) admitLink(w http.ResponseWriter, r *http.Request, linkID, secret string) (*store.PreviewLink, bool) {
	row, err := s.db.LinkByID(r.Context(), linkID)
	if err != nil {
		httpx.NotFound(w, r) // unknown id (spec §30: don't leak existence)
		return nil, false
	}
	// Constant-time secret comparison (spec §14/§15).
	if !hash.SecretEqual(secret, row.SecretHashOf()) {
		httpx.NotFound(w, r)
		return nil, false
	}
	if row.RevokedAt != nil {
		httpx.Gone(w, r, "link revoked")
		return nil, false
	}
	if row.FrozenAt != nil {
		// Frozen links surface an appeal path (spec §9/§15.5).
		httpx.Err(w, r, http.StatusGone, httpx.CodeGone, "link frozen for review; contact support to appeal", false)
		return nil, false
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		httpx.Gone(w, r, "link expired")
		return nil, false
	}
	return &row.PreviewLink, true
}

// resolveBacking loads the mime/displayName/backing-expiry for a link.
func (s *Server) resolveBacking(r *http.Request, link *store.PreviewLink) (mime, name string, backingExpiry *time.Time, err error) {
	switch link.Mode {
	case "tunnel":
		fr, e := s.db.FileRefByID(r.Context(), *link.FileRef)
		if e != nil {
			return "", "", nil, errGone
		}
		return fr.MimeType, fr.DisplayName, fr.ExpiresAt, nil
	case "cloud":
		f, e := s.db.FileByIDAny(r.Context(), *link.FileID)
		if e != nil {
			return "", "", nil, errGone
		}
		if f.Status != "ready" {
			return "", "", nil, errGone
		}
		return f.MimeType, f.DisplayName, f.ExpiresAt, nil
	}
	return "", "", nil, errGone
}

var errGone = errors.New("gone")

func (s *Server) serveError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errGone) {
		httpx.Gone(w, r, "content unavailable")
		return
	}
	httpx.Internal(w, r)
}

// serveCloud streams a cloud object with range support (spec §11/§13), or
// redirects to a presigned URL to offload bytes off the API (spec §19.3) when a
// browser-reachable endpoint is configured and the link has no per-view cap.
func (s *Server) serveCloud(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, name string, allowDownload, forceAttachment, allowRedirect bool) {
	f, err := s.db.FileByIDAny(r.Context(), *link.FileID)
	if err != nil {
		httpx.Gone(w, r, "file unavailable")
		return
	}
	// Offload large benign content to a signed URL. Skipped for view-capped links
	// (a 15-min URL must not be replayable past maxViews) and active content.
	if allowRedirect && s.cfg.S3PublicEndpoint != "" && !forceAttachment {
		switch previewCategory(f.MimeType) {
		case "image", "pdf":
			if signed, err := s.store.PresignGet(f.StorageKey, name); err == nil {
				s.meterEgress(link.TenantID, "cloud", f.Size) // the object store serves the bytes
				http.Redirect(w, r, signed, http.StatusFound)
				return
			}
		}
	}
	body, ctype, size, err := s.store.Get(f.StorageKey)
	if err != nil {
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "storage unavailable", true)
		return
	}
	defer body.Close()
	if ctype == "" {
		ctype = f.MimeType
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Accept-Ranges", "bytes")
	s.setDownloadHeaders(w, name, allowDownload, forceAttachment)
	http.ServeContent(w, r, name, f.CreatedAt, rsAdapter{body, size})
	_ = size
}

// setDownloadHeaders writes Content-Disposition (spec §15). Active content
// (forceAttachment) is served as an attachment so the browser never renders it
// inline. Viewable content stays inline; note that download_disabled (spec §14)
// is still best-effort here — true prevention needs signed-URL gating (P1).
func (s *Server) setDownloadHeaders(w http.ResponseWriter, name string, allowDownload, forceAttachment bool) {
	disp := "inline"
	if forceAttachment {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disp, name, name))
}

// --- account / password / unlock helpers ---

func (s *Server) accountAllowed(r *http.Request, pol store.AccessPolicy) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	userID, err := s.db.SessionUser(r.Context(), c.Value)
	if err != nil {
		return false
	}
	if pol.Account == nil {
		return true // logged in is enough
	}
	for _, uid := range pol.Account.AllowedUserIDs {
		if uid == userID {
			return true
		}
	}
	if len(pol.Account.AllowedTenantIDs) > 0 {
		ms, _ := s.db.MembershipsForUser(r.Context(), userID)
		for _, m := range ms {
			for _, tid := range pol.Account.AllowedTenantIDs {
				if m.TenantID == tid {
					return true
				}
			}
		}
		return false
	}
	return len(pol.Account.AllowedUserIDs) == 0 // no explicit list => any logged-in user
}

func unlockCookie(linkID string) string { return "apage_unlock_" + linkID }

func (s *Server) unlockToken(linkID string) string {
	return hash.SecretHash(linkID + ":" + s.cfg.SessionSecret)
}

func (s *Server) hasUnlock(r *http.Request, linkID string) bool {
	c, err := r.Cookie(unlockCookie(linkID))
	if err != nil {
		return false
	}
	// cookie value == unlockToken(linkID) == SecretHash(linkID:sessionSecret).
	// Verify in constant time by hashing the preimage and comparing to the cookie.
	return hash.SecretEqual(linkID+":"+s.cfg.SessionSecret, c.Value)
}

// --- async stat / audit (spec §19.7 final consistency) ---

func (s *Server) flushViewCount(linkID string, n int64) {
	_ = s.db.TouchLinkAccess(context.Background(), linkID, n)
}

func (s *Server) access(linkID, tenantID, instanceID string, r *http.Request) {
	s.audit(context.Background(), audit.Entry{TenantID: tenantID, InstanceID: instanceID,
		Event: audit.PreviewLinkAccessed, ActorType: audit.ActorAnonymous,
		ResourceType: "preview_link", ResourceID: linkID,
		IP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent()})
}

func (s *Server) deny(r *http.Request, link *store.PreviewLink, reason string) {
	s.audit(context.Background(), audit.Entry{TenantID: link.TenantID, InstanceID: link.InstanceID,
		Event: audit.PreviewLinkDenied, ActorType: audit.ActorAnonymous,
		ResourceType: "preview_link", ResourceID: link.LinkID, IP: httpx.ClientIP(r.Context()), Reason: reason})
}

// rsAdapter adapts an object body to io.ReadSeeker for http.ServeContent.
type rsAdapter struct {
	rsc  io.ReadSeekCloser
	size int64
}

func (a rsAdapter) Read(p []byte) (int, error)                { return a.rsc.Read(p) }
func (a rsAdapter) Seek(off int64, whence int) (int64, error) { return a.rsc.Seek(off, whence) }

func headersSent(http.ResponseWriter) bool {
	// chi/std lib do not expose this; treat as best-effort false.
	return false
}

// passwordPageHTML renders the minimal password gate (spec §9). Server-rendered,
// no scripts beyond a tiny inline fetch; CSP for text applies.
func passwordPageHTML(linkID, secret string, retry bool) string {
	msg := ""
	if retry {
		msg = `<p style="color:#DC2626">Incorrect password.</p>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected preview</title></head>
<body style="font-family:system-ui;max-width:360px;margin:15vh auto;padding:0 16px">
<h2>This preview is password protected</h2>` + msg + `
<form method="POST" action="/p/` + linkID + `/` + secret + `/unlock" onsubmit="return submitPw(event)">
<input id="pw" type="password" placeholder="Password" style="width:100%;padding:10px;font-size:16px" autofocus>
<button style="margin-top:12px;width:100%;padding:10px;font-size:16px">Unlock</button>
</form>
<p style="color:#5B6472;font-size:13px;margin-top:16px">Don't paste this link into public channels.</p>
<script>
async function submitPw(e){e.preventDefault();
 const res=await fetch(e.target.action,{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({password:document.getElementById('pw').value})});
 if(res.ok){location.reload();}else{document.getElementById('pw').value='';alert('Incorrect password');}
 return false;}
</script></body></html>`
}

var _ io.ReadSeeker = rsAdapter{}
