package api

import (
	"net/http"
	"strings"
)

// previewCategory classifies a mime type into a rendering category (spec §13).
func previewCategory(mime string) string {
	mime = strings.ToLower(mime)
	switch {
	case mime == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mime, "image/svg"):
		return "svg" // high risk (spec §13)
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case mime == "text/html":
		return "html" // high risk (spec §13)
	case strings.HasPrefix(mime, "text/"),
		mime == "application/json", mime == "application/xml",
		strings.Contains(mime, "csv"), strings.Contains(mime, "log"):
		return "text"
	default:
		return "binary"
	}
}

// setSecurityHeaders applies the baseline + per-type CSP from spec §15.
// All types keep frame-ancestors 'none' and nosniff.
func setSecurityHeaders(w http.ResponseWriter, category string) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "private, max-age=0, no-store")
	h.Set("Cross-Origin-Resource-Policy", "same-site")

	var csp string
	switch category {
	case "image":
		csp = "default-src 'none'; img-src 'self'; sandbox; frame-ancestors 'none'"
	case "text":
		csp = "default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self'; frame-ancestors 'none'"
	case "pdf":
		csp = "default-src 'none'; object-src 'self'; frame-src 'self'; img-src 'self'; frame-ancestors 'none'"
	case "html", "svg":
		// High-risk: rendered only on the isolated render domain with a
		// sandboxed iframe lacking allow-scripts/allow-same-origin (spec §13/§15).
		csp = "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; frame-ancestors 'none'; sandbox"
	default:
		csp = "default-src 'none'; frame-ancestors 'none'; sandbox"
	}
	h.Set("Content-Security-Policy", csp)
}
