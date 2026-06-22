package api

import (
	"crypto/subtle"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apage/apage/internal/hash"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// Active content (HTML/SVG) can execute script, so it is never rendered inline on
// the control plane (spec §13/§15.5). The flow:
//
//  1. The canonical link on the preview subdomain enforces ip/account/password
//     with the visitor's cookies, then redirects to the isolated render origin
//     with a short-lived, unguessable grant (the render origin is cookie-less).
//  2. On the render origin, a sandboxed wrapper page frames the bytes: HTML in an
//     <iframe sandbox> (no allow-scripts/allow-same-origin), SVG as an <img> (an
//     image context cannot run script). The wrapper consumes no view.
//  3. The /raw subresource validates the grant, re-checks ip, consumes the view,
//     and streams the bytes under a strict, script-free CSP.
//
// The grant binds only the link + a ~10-minute window, so a render-origin URL is
// not a durable capability; the path secret remains the primary capability.

// serveActive handles an HTML/SVG link (called from handlePreview).
func (s *Server) serveActive(w http.ResponseWriter, r *http.Request, link *store.PreviewLink, pol store.AccessPolicy, category, displayName string, backingExpiry *time.Time) {
	linkID := link.LinkID
	secret := chi.URLParam(r, "secret")
	rd := s.cfg.RenderDomain

	// Control plane: enforce gates with cookies, then bounce to the render origin.
	if rd != "" && !s.onRenderDomain(r) {
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
		dest := "https://" + rd + "/p/" + linkID + "/" + secret + "?g=" + url.QueryEscape(s.issueRenderGrant(linkID))
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}

	// Render origin (or isolation disabled): authorize, then serve the wrapper.
	grant := r.URL.Query().Get("g")
	if rd != "" {
		if !s.validRenderGrant(linkID, grant) {
			httpx.Forbidden(w, r, "open this preview from its original link")
			return
		}
		if !s.gateIP(w, r, link, pol) { // ip is still verifiable here
			return
		}
	} else {
		// RenderDomain unset (dev): no separate origin, so gate with cookies here.
		if !s.gateIP(w, r, link, pol) || !s.gateAccount(w, r, link, pol) {
			return
		}
		if passwordRequired(pol) && !s.hasUnlock(r, linkID) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			setSecurityHeaders(w, "text")
			_, _ = w.Write([]byte(passwordPageHTML(linkID, secret, false)))
			return
		}
	}
	if s.backingExpired(link, backingExpiry) {
		httpx.Gone(w, r, "link expired")
		return
	}
	setWrapperHeaders(w)
	_, _ = w.Write([]byte(renderWrapperHTML(linkID, secret, category, displayName, pol.AllowDownload, grant)))
}

// handlePreviewRaw streams the bytes of an active-content link for embedding by
// the sandboxed wrapper. This is where the view is consumed (spec §13/§15.5).
func (s *Server) handlePreviewRaw(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkId")
	secret := chi.URLParam(r, "secret")
	if !s.limit(w, r, "preview:"+httpx.ClientIP(r.Context()), 600, time.Minute) {
		return
	}
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
	if !isActiveContent(previewCategory(mime)) {
		httpx.NotFound(w, r) // /raw is only for sandboxed active content
		return
	}
	rd := s.cfg.RenderDomain
	if rd != "" {
		if !s.onRenderDomain(r) || !s.validRenderGrant(linkID, r.URL.Query().Get("g")) {
			httpx.Forbidden(w, r, "open this preview from its original link")
			return
		}
		if !s.gateIP(w, r, link, pol) {
			return
		}
	} else {
		if !s.gateIP(w, r, link, pol) || !s.gateAccount(w, r, link, pol) {
			return
		}
		if passwordRequired(pol) && !s.hasUnlock(r, linkID) {
			httpx.Forbidden(w, r, "locked")
			return
		}
	}
	if s.backingExpired(link, backingExpiry) {
		httpx.Gone(w, r, "link expired")
		return
	}
	if !s.consumeView(w, r, link, pol) {
		return
	}
	go s.access(linkID, link.TenantID, link.InstanceID, r)
	setRawActiveHeaders(w)
	download := r.URL.Query().Get("dl") == "1" && pol.AllowDownload
	// Active content must stay on the render origin under our CSP — never redirect.
	s.serveBytes(w, r, link, displayName, pol.AllowDownload, download, false)
}

// onRenderDomain reports whether the request arrived on the isolated render
// origin. When RenderDomain is unset (dev), isolation is disabled and this is
// treated as true so previews still work on a single origin.
func (s *Server) onRenderDomain(r *http.Request) bool {
	rd := s.cfg.RenderDomain
	if rd == "" {
		return true
	}
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return host == rd
}

// renderGrant is a deterministic HMAC-like token over (link, time window). It is
// unguessable without SessionSecret and self-expires with the window.
func (s *Server) renderGrant(linkID string, bucket int64) string {
	return hash.SecretHash(fmt.Sprintf("rendergrant:%s:%d:%s", linkID, bucket, s.cfg.SessionSecret))
}

func (s *Server) issueRenderGrant(linkID string) string {
	return s.renderGrant(linkID, time.Now().Unix()/600)
}

// validRenderGrant accepts the current and previous 10-minute window so a grant
// stays valid for up to ~20 minutes (constant-time comparison).
func (s *Server) validRenderGrant(linkID, g string) bool {
	if g == "" {
		return false
	}
	now := time.Now().Unix() / 600
	for _, b := range []int64{now, now - 1} {
		if subtle.ConstantTimeCompare([]byte(g), []byte(s.renderGrant(linkID, b))) == 1 {
			return true
		}
	}
	return false
}

// setWrapperHeaders secures the outer wrapper page (spec §15). It may only frame
// same-origin content and cannot itself be framed.
func setWrapperHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "private, max-age=0, no-store")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Content-Security-Policy",
		"default-src 'none'; frame-src 'self'; img-src 'self' data: blob:; style-src 'unsafe-inline'; frame-ancestors 'none'")
}

// setRawActiveHeaders secures the embedded document itself: no script at any
// layer, framable only by our same-origin wrapper (spec §13/§15.5).
func setRawActiveHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "private, max-age=0, no-store")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; script-src 'none'; sandbox; frame-ancestors 'self'")
}

// renderWrapperHTML builds the sandboxed viewer. HTML is framed with a fully
// restrictive sandbox (no scripts, no same-origin); SVG is shown as an image.
func renderWrapperHTML(linkID, secret, category, displayName string, allowDownload bool, grant string) string {
	q := ""
	if grant != "" {
		q = "?g=" + url.QueryEscape(grant)
	}
	raw := "/p/" + linkID + "/" + secret + "/raw" + q
	name := html.EscapeString(displayName)
	dl := ""
	if allowDownload {
		sep := "?"
		if q != "" {
			sep = "&"
		}
		dl = `<a class="dl" href="` + html.EscapeString(raw+sep+"dl=1") + `" download>Download</a>`
	}
	var viewer string
	if category == "svg" {
		viewer = `<img src="` + html.EscapeString(raw) + `" alt="` + name + `">`
	} else {
		viewer = `<iframe src="` + html.EscapeString(raw) + `" sandbox referrerpolicy="no-referrer"></iframe>`
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>` + name + `</title>
<style>
*{box-sizing:border-box}html,body{margin:0;height:100%;font-family:system-ui,sans-serif;background:#0b0d10;color:#e6e8eb}
header{display:flex;align-items:center;justify-content:space-between;gap:12px;height:48px;padding:0 16px;background:#15181d;border-bottom:1px solid #262b33}
.name{font-size:14px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.dl{font-size:13px;color:#9ecbff;text-decoration:none;border:1px solid #2d6cdf;border-radius:6px;padding:5px 12px}
main{height:calc(100% - 48px);overflow:auto;display:flex;justify-content:center;background:#fff}
iframe{border:0;width:100%;height:100%}
img{max-width:100%;height:auto;display:block;margin:auto}
</style></head>
<body><header><span class="name">` + name + `</span>` + dl + `</header>
<main>` + viewer + `</main></body></html>`
}
