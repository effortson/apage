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
// trustForwarded should be true only when the service sits behind our own edge.
func RequestContext(trustForwarded bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get("X-Request-Id")
			if rid == "" {
				rid = id.New(id.PrefixRequest)
			}
			ip := clientIP(r, trustForwarded)
			ctx := context.WithValue(r.Context(), ctxRequestID, rid)
			ctx = context.WithValue(ctx, ctxClientIP, ip)
			w.Header().Set("X-Request-Id", rid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clientIP(r *http.Request, trustForwarded bool) string {
	if trustForwarded {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
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
