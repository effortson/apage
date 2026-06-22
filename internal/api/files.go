package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

// etagMatches compares ETags ignoring surrounding quotes and case (S3 ETags for
// single-part PUTs are the hex MD5; minio returns them quoted).
func etagMatches(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Trim(strings.TrimSpace(s), `"`)) }
	return norm(a) == norm(b)
}

// allowedMIME is the upload MIME allowlist (spec §13/§15 MIME sniffing).
var allowedMIME = map[string]bool{
	"application/pdf": true,
	"image/png":       true, "image/jpeg": true, "image/webp": true, "image/gif": true,
	"text/plain": true, "text/markdown": true, "application/json": true,
	"text/csv": true, "text/html": true,
}

func storageKey(tenantID, instanceID, fileID string) string {
	return tenantID + "/" + instanceID + "/" + fileID + "/original"
}

// handleUploadFile is the small-file direct multipart path (spec §12).
// Files larger than DirectUploadMaxBytes must use presign (413).
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	if s.store == nil {
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "cloud storage disabled", false)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.DirectUploadMaxBytes+1024)
	if err := r.ParseMultipartForm(s.cfg.DirectUploadMaxBytes); err != nil {
		httpx.Err(w, r, http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge,
			"file exceeds direct-upload limit; use /uploads/presign", false)
		return
	}
	instanceID, ok := s.opInstanceID(w, r, sc, r.FormValue("instanceId"))
	if !ok {
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, r, "missing file field")
		return
	}
	defer file.Close()
	if hdr.Size > s.cfg.DirectUploadMaxBytes {
		httpx.Err(w, r, http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge,
			"file too large for direct upload; use presign", false)
		return
	}
	mime := hdr.Header.Get("Content-Type")
	if !allowedMIME[mime] {
		httpx.Err(w, r, http.StatusUnsupportedMediaType, httpx.CodeUnsupportedType, "unsupported mime type", false)
		return
	}
	// Quota check (spec §2 用量校验点).
	if q, err := s.db.QuotaFor(r.Context(), sc.TenantID); err == nil {
		if q.StorageBytesUsed+hdr.Size > q.StorageBytesLimit {
			httpx.QuotaExceeded(w, r, "storage limit exceeded")
			return
		}
	}
	displayName := r.FormValue("displayName")
	if displayName == "" {
		displayName = hdr.Filename
	}
	exp := s.clampExpiryToPlan(r, sc.TenantID, s.fileExpiry(r.FormValue("expiresInSeconds")))

	fileID := id.New(id.PrefixFile)
	key := storageKey(sc.TenantID, instanceID, fileID)
	f := store.File{
		FileID: fileID, TenantID: sc.TenantID, InstanceID: instanceID,
		Status: "uploaded", PreviewStatus: "pending", DisplayName: displayName,
		Size: hdr.Size, MimeType: mime, StorageKey: key, Visibility: "private", ExpiresAt: exp,
	}
	if err := s.db.CreateFile(r.Context(), f); err != nil {
		httpx.Internal(w, r)
		return
	}
	// Stream into object storage. Direct upload proxies once; large files use
	// presign to keep bytes off the API service (spec §19.3).
	if err := s.store.Put(key, mime, file); err != nil {
		s.log.Error("put object", "err", err)
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "storage unavailable", true)
		return
	}
	_ = s.db.SetFileStatus(r.Context(), fileID, "scanning", "pending", "")
	_ = s.db.AddStorageUsed(r.Context(), sc.TenantID, hdr.Size)
	_ = s.rdb.Enqueue(r.Context(), "scan", fileID)
	s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID, InstanceID: instanceID,
		Event: audit.FileUploaded, ActorType: actorOf(sc), ActorID: actorID(sc),
		ResourceType: "file", ResourceID: fileID})

	// Initial status is never "ready" (spec §12 / P0-1).
	httpx.JSON(w, http.StatusOK, map[string]any{
		"fileId": fileID, "status": "scanning", "previewStatus": "pending", "expiresAt": exp,
	})
}

// opInstanceID resolves the instance to attribute an upload to: the instance-key
// scope uses its own; a console session must name a tenant-owned instance.
func (s *Server) opInstanceID(w http.ResponseWriter, r *http.Request, sc *dataScope, fallback string) (string, bool) {
	if sc.Instance != nil {
		return sc.Instance.InstanceID, true
	}
	if fallback == "" {
		httpx.BadRequest(w, r, "instanceId required")
		return "", false
	}
	in, err := s.db.InstanceByID(r.Context(), fallback)
	if err != nil || in.TenantID != sc.TenantID {
		httpx.NotFound(w, r)
		return "", false
	}
	return in.InstanceID, true
}

type presignReq struct {
	FileName         string `json:"fileName"`
	MimeType         string `json:"mimeType"`
	Size             int64  `json:"size"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
	InstanceID       string `json:"instanceId"` // required for console session uploads
}

// handlePresign issues a presigned upload URL after validating quota/size/MIME (spec §12).
func (s *Server) handlePresign(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	if s.store == nil {
		httpx.Err(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "cloud storage disabled", false)
		return
	}
	var req presignReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	if req.Size <= 0 {
		httpx.BadRequest(w, r, "size required")
		return
	}
	if !allowedMIME[req.MimeType] {
		httpx.Err(w, r, http.StatusUnsupportedMediaType, httpx.CodeUnsupportedType, "unsupported mime type", false)
		return
	}
	instanceID, ok := s.opInstanceID(w, r, sc, req.InstanceID)
	if !ok {
		return
	}
	if q, err := s.db.QuotaFor(r.Context(), sc.TenantID); err == nil {
		if req.Size > q.StorageBytesLimit || q.StorageBytesUsed+req.Size > q.StorageBytesLimit {
			httpx.QuotaExceeded(w, r, "storage limit exceeded")
			return
		}
	}
	s.idempotent(sc.idemScope(), "presign", bodyHash(req), w, r, func() (int, any) {
		fileID := id.New(id.PrefixFile)
		key := storageKey(sc.TenantID, instanceID, fileID)
		f := store.File{
			FileID: fileID, TenantID: sc.TenantID, InstanceID: instanceID,
			Status: "uploading", PreviewStatus: "pending", DisplayName: req.FileName,
			Size: req.Size, MimeType: req.MimeType, StorageKey: key, Visibility: "private",
			ExpiresAt: s.clampExpiryToPlan(r, sc.TenantID, s.fileExpirySeconds(req.ExpiresInSeconds)),
		}
		if err := s.db.CreateFile(r.Context(), f); err != nil {
			return 500, internalBody(r)
		}
		url, headers, err := s.store.PresignPut(key, req.MimeType)
		if err != nil {
			return 500, internalBody(r)
		}
		return http.StatusOK, map[string]any{"uploadUrl": url, "fileId": fileID, "headers": headers}
	})
}

type completeReq struct {
	Etag   string `json:"etag"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

// handleCompleteUpload finalizes a presigned upload and queues scanning (spec §12).
func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	fileID := chi.URLParam(r, "fileId")
	f, err := s.db.FileByID(r.Context(), sc.TenantID, fileID)
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	if f.Status != "uploading" {
		httpx.Conflict(w, r, "file is not awaiting completion (status="+f.Status+")")
		return
	}
	var req completeReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid body")
		return
	}
	// Verify the presigned upload actually landed and matches the claim before
	// trusting client-supplied size (spec §12 integrity). The stored object's
	// size is authoritative; an absent object means the upload never completed.
	size := req.Size
	if size <= 0 {
		size = f.Size
	}
	if s.store != nil {
		actualSize, etag, err := s.store.Stat(f.StorageKey)
		if err != nil {
			httpx.Conflict(w, r, "uploaded object not found; complete only after the upload succeeds")
			return
		}
		if req.Etag != "" && !etagMatches(req.Etag, etag) {
			httpx.Err(w, r, http.StatusConflict, httpx.CodeConflict, "etag mismatch: uploaded object differs from completion claim", false)
			return
		}
		size = actualSize // authoritative; ignore a spoofed client size
	}
	// Re-check the quota against the object's real size. The presigned PUT URL
	// does not enforce the size promised at /presign, so an oversized object that
	// would breach the tenant's storage quota is rejected here and its bytes are
	// scheduled for deletion rather than silently counted (storage-quota bypass).
	if q, err := s.db.QuotaFor(r.Context(), sc.TenantID); err == nil {
		if q.StorageBytesUsed+size > q.StorageBytesLimit {
			_ = s.db.SetFileStatus(r.Context(), fileID, "rejected", "failed", "storage quota exceeded")
			if f.StorageKey != "" {
				_ = s.rdb.Enqueue(r.Context(), "delete", f.StorageKey)
			}
			httpx.QuotaExceeded(w, r, "storage limit exceeded")
			return
		}
	}
	if err := s.db.FinalizeUpload(r.Context(), fileID, size); err != nil {
		httpx.Internal(w, r)
		return
	}
	_ = s.db.AddStorageUsed(r.Context(), sc.TenantID, size)
	_ = s.rdb.Enqueue(r.Context(), "scan", fileID)
	s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID, InstanceID: f.InstanceID,
		Event: audit.FileUploaded, ActorType: actorOf(sc), ActorID: actorID(sc),
		ResourceType: "file", ResourceID: fileID})
	httpx.JSON(w, http.StatusOK, map[string]any{"fileId": fileID, "status": "scanning", "previewStatus": "pending"})
}

// handleGetFile returns file status (spec §12).
func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "viewer")
	if sc == nil {
		return
	}
	f, err := s.db.FileByID(r.Context(), sc.TenantID, chi.URLParam(r, "fileId"))
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, f)
}

// handleListFiles lists cloud files (spec §14).
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "viewer")
	if sc == nil {
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	q := r.URL.Query()
	items, err := s.db.ListFiles(r.Context(), sc.TenantID, p, q.Get("status"), q.Get("instanceId"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(f store.File) (time.Time, string) {
		return f.CreatedAt, f.FileID
	}))
}

// handleDeleteFile marks a file deleted and queues object cleanup (spec §11).
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	sc := scopeRole(w, r, "member")
	if sc == nil {
		return
	}
	fileID := chi.URLParam(r, "fileId")
	f, err := s.db.MarkFileDeleted(r.Context(), sc.TenantID, fileID)
	if err != nil {
		httpx.NotFound(w, r)
		return
	}
	// Cascade: invalidate all links backed by this file (spec §11).
	if links, err := s.db.LinksByFile(r.Context(), fileID); err == nil {
		for _, lid := range links {
			_ = s.rdb.InvalidateLink(r.Context(), lid)
		}
	}
	_ = s.db.AddStorageUsed(r.Context(), sc.TenantID, -f.Size)
	_ = s.rdb.Enqueue(r.Context(), "delete", f.StorageKey)
	s.audit(r.Context(), audit.Entry{TenantID: sc.TenantID, InstanceID: f.InstanceID,
		Event: audit.FileDeleted, ActorType: actorOf(sc), ActorID: actorID(sc),
		ResourceType: "file", ResourceID: fileID})
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- helpers ---

func (s *Server) fileExpiry(secStr string) *time.Time {
	if secStr == "" {
		return s.defaultFileExpiry()
	}
	var sec int64
	for _, c := range secStr {
		if c < '0' || c > '9' {
			return s.defaultFileExpiry()
		}
		sec = sec*10 + int64(c-'0')
	}
	return s.fileExpirySeconds(sec)
}

func (s *Server) fileExpirySeconds(sec int64) *time.Time {
	if sec <= 0 {
		return s.defaultFileExpiry()
	}
	t := time.Now().Add(time.Duration(sec) * time.Second)
	return &t
}

// defaultFileExpiry applies the lite 24h retention default (spec §20).
func (s *Server) defaultFileExpiry() *time.Time {
	t := time.Now().Add(24 * time.Hour)
	return &t
}

// clampExpiryToPlan caps a requested expiry to the tenant plan's max retention
// so a lite tenant cannot set a 30-day expiry (spec §20 lite boundary).
func (s *Server) clampExpiryToPlan(r *http.Request, tenantID string, exp *time.Time) *time.Time {
	t, err := s.db.TenantByID(r.Context(), tenantID)
	if err != nil {
		return exp
	}
	max := planMaxLinkTTL(t.Plan)
	if max == 0 {
		return exp
	}
	cap := time.Now().Add(max)
	if exp == nil || exp.After(cap) {
		return &cap
	}
	return exp
}
