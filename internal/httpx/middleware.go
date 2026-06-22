package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apage/apage/internal/id"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxClientIP
)

// RequestID returns the request id stored in context, or "".
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

// ClientIP returns the trusted client IP stored in context.
// Spec §14: only trust X-Forwarded-For injected by our own edge.
func ClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(ctxClientIP).(string); ok {
		return v
	}
	return ""
}

// RequestContext assigns a request id and resolves the trusted client IP.
// trustedProxyCount is the number of trusted reverse proxies in front of the
// service; 0 means do not trust X-Forwarded-For at all.
func RequestContext(trustedProxyCount int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get("X-Request-Id")
			if rid == "" {
				rid = id.New(id.PrefixRequest)
			}
			ip := clientIP(r, trustedProxyCount)
			ctx := context.WithValue(r.Context(), ctxRequestID, rid)
			ctx = context.WithValue(ctx, ctxClientIP, ip)
			w.Header().Set("X-Request-Id", rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIP resolves the real client IP. With N trusted proxies, the client IP
// is the entry N hops from the right of X-Forwarded-For (our own edge appends
// the true peer there). A client-spoofed value sits to the left and is ignored.
// Falls back to RemoteAddr when X-Forwarded-For is absent or too short to trust.
func clientIP(r *http.Request, trustedProxyCount int) string {
	remote := r.RemoteAddr
	if i := strings.LastIndex(remote, ":"); i > 0 {
		remote = remote[:i]
	}
	remote = strings.Trim(remote, "[]") // strip IPv6 brackets
	if trustedProxyCount <= 0 {
		return remote
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote
	}
	parts := strings.Split(xff, ",")
	idx := len(parts) - trustedProxyCount
	if idx < 0 || idx >= len(parts) {
		// Fewer hops than expected: the request didn't traverse our full proxy
		// chain, so the header is untrustworthy — use the direct peer.
		return remote
	}
	if v := strings.TrimSpace(parts[idx]); v != "" {
		return v
	}
	return remote
}

// Logger logs each request with method, path, status, and duration.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)
			log.Info("http",
				"method", r.Method,
				"path", scrubSecret(r.URL.Path),
				"status", sw.status,
				"dur_ms", time.Since(start).Milliseconds(),
				"request_id", RequestID(r.Context()),
			)
		})
	}
}

// scrubSecret redacts the secret path segment from /p/{linkId}/{secret} and
// /f/{fileId}/{secret} URLs in logs (spec §8: secret must never appear in logs).
func scrubSecret(path string) string {
	segs := strings.Split(path, "/")
	for i := 1; i < len(segs)-1; i++ {
		if (segs[i] == "p" || segs[i] == "f") && i+2 < len(segs) {
			segs[i+2] = "***"
		}
	}
	return strings.Join(segs, "/")
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher for streaming responses.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Recover converts panics into 500 responses.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "path", scrubSecret(r.URL.Path))
					Internal(w, r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SetRateLimitHeaders writes the standard rate-limit headers (spec §"限流响应约定").
func SetRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetUnix int64) {
	w.Header().Set("RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("RateLimit-Reset", strconv.FormatInt(resetUnix, 10))
}

// TooManyRequests writes a 429 with Retry-After (spec §"限流响应约定").
func TooManyRequests(w http.ResponseWriter, r *http.Request, retryAfterSec int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	Err(w, r, http.StatusTooManyRequests, CodeRateLimited, "rate limit exceeded", true)
}
