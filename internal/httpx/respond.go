// Package httpx implements the shared HTTP conventions from spec §"统一 API 约定":
// error envelope, cursor pagination, idempotency, rate-limit headers, and
// request context middleware.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorEnvelope is the universal error response shape (spec §"通用错误响应").
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries the machine-readable error detail.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Retryable bool   `json:"retryable"`
}

// Error codes (spec §7 / §"通用错误响应").
const (
	CodeBadRequest         = "BAD_REQUEST"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeAccessDenied       = "ACCESS_DENIED"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeGone               = "GONE"
	CodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
	CodeUnsupportedType    = "UNSUPPORTED_TYPE"
	CodeRateLimited        = "RATE_LIMITED"
	CodeInternal           = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeQuotaExceeded      = "QUOTA_EXCEEDED"
	CodeNotReady           = "NOT_READY"
)

// JSON writes a JSON body with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Err writes a standard error envelope. retryable hints clients on whether a
// retry may succeed.
func Err(w http.ResponseWriter, r *http.Request, status int, code, msg string, retryable bool) {
	JSON(w, status, ErrorEnvelope{Error: ErrorBody{
		Code:      code,
		Message:   msg,
		RequestID: RequestID(r.Context()),
		Retryable: retryable,
	}})
}

// Common shorthands.
func BadRequest(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusBadRequest, CodeBadRequest, msg, false)
}
func Unauthorized(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusUnauthorized, CodeUnauthorized, msg, false)
}
func Forbidden(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusForbidden, CodeForbidden, msg, false)
}
func NotFound(w http.ResponseWriter, r *http.Request) {
	Err(w, r, http.StatusNotFound, CodeNotFound, "resource not found", false)
}
func Conflict(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusConflict, CodeConflict, msg, false)
}
func Gone(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusGone, CodeGone, msg, false)
}
func Internal(w http.ResponseWriter, r *http.Request) {
	Err(w, r, http.StatusInternalServerError, CodeInternal, "internal server error", true)
}
func QuotaExceeded(w http.ResponseWriter, r *http.Request, msg string) {
	Err(w, r, http.StatusForbidden, CodeQuotaExceeded, msg, false)
}

// DecodeJSON decodes a request body, rejecting unknown fields.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
