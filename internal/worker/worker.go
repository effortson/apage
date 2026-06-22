// Package worker runs async jobs: virus/MIME scanning, preview status, expiry
// sweeping, and object deletion with retry (spec §10, §11, §19.3).
package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/redisx"
	"github.com/apage/apage/internal/store"
)

// ObjectStore is the subset of the object store the worker needs: deleting
// objects and reading the leading bytes for content sniffing (spec §10/§11).
type ObjectStore interface {
	Delete(keys ...string) error
	Get(key string) (body io.ReadSeekCloser, contentType string, size int64, err error)
}

// Worker drains queues and runs periodic sweeps.
type Worker struct {
	db  *store.Store
	rdb *redisx.Client
	obj ObjectStore
	log *slog.Logger
}

// New builds a worker.
func New(db *store.Store, rdb *redisx.Client, obj ObjectStore, log *slog.Logger) *Worker {
	return &Worker{db: db, rdb: rdb, obj: obj, log: log}
}

// Run starts the queue consumers and sweep loop until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	go w.consume(ctx, "scan", w.handleScan)
	go w.consume(ctx, "delete", w.handleDelete)
	go w.consume(ctx, "audit", w.handleAudit)
	go w.sweepLoop(ctx)
	go w.usageFlushLoop(ctx)
	go w.domainRecheckLoop(ctx)
	<-ctx.Done()
}

// domainRecheckLoop periodically re-verifies custom-domain ownership and reverts
// domains whose TXT record has disappeared (spec §28 定期检查).
func (w *Worker) domainRecheckLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.recheckDomains(ctx)
		}
	}
}

func (w *Worker) recheckDomains(ctx context.Context) {
	domains, err := w.db.DomainsToRecheck(ctx, time.Now().Add(-12*time.Hour), 50)
	if err != nil {
		w.log.Warn("domain recheck list", "err", err)
		return
	}
	for _, d := range domains {
		if txtRecordPresent(d.Domain, d.TXTValue) {
			_ = w.db.SetDomainStatus(ctx, d.DomainID, d.Status, d.CertStatus) // refresh last_checked_at
			continue
		}
		// Ownership TXT gone: revert to failed and audit (spec §28).
		_ = w.db.SetDomainStatus(ctx, d.DomainID, "failed", "none")
		_ = w.db.WriteAudit(ctx, audit.Entry{TenantID: d.TenantID, Event: audit.CustomDomainFailed,
			ActorType: audit.ActorSystem, ResourceType: "custom_domain", ResourceID: d.DomainID, Reason: "txt_record_missing"})
		w.log.Info("custom domain reverted (txt missing)", "domain", d.Domain)
	}
}

// txtRecordPresent reports whether the ownership TXT record is still published.
func txtRecordPresent(domain, expected string) bool {
	records, err := net.LookupTXT("_apage." + domain)
	if err != nil {
		return false
	}
	for _, rec := range records {
		if strings.TrimSpace(rec) == expected {
			return true
		}
	}
	return false
}

// usageFlushLoop periodically drains the Redis usage buffer to the DB so the hot
// path never writes the quotas table directly (spec §19.7 metering).
func (w *Worker) usageFlushLoop(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.flushUsage(ctx)
		}
	}
}

func (w *Worker) flushUsage(ctx context.Context) {
	deltas, err := w.rdb.DrainUsage(ctx)
	if err != nil {
		w.log.Warn("usage drain", "err", err)
		return
	}
	for _, d := range deltas {
		if err := w.db.AddUsage(ctx, d.TenantID, d.Dim, d.N); err != nil {
			w.log.Warn("usage flush", "dim", d.Dim, "err", err)
			continue
		}
		_ = w.db.AddUsageDaily(ctx, d.TenantID, d.Dim, d.N)
	}
}

// handleAudit persists an audit entry enqueued by the API off the request path
// (spec §15/§19.7 async audit).
func (w *Worker) handleAudit(ctx context.Context, payload string) {
	var e audit.Entry
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return
	}
	if err := w.db.WriteAudit(ctx, e); err != nil {
		w.log.Warn("audit write", "event", e.Event, "err", err)
	}
}

func (w *Worker) consume(ctx context.Context, queue string, fn func(context.Context, string)) {
	for ctx.Err() == nil {
		payload, err := w.rdb.Dequeue(ctx, queue, 5*time.Second)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if payload == "" {
			continue
		}
		fn(ctx, payload)
	}
}

// handleScan runs the scan stage and advances the file to ready/rejected
// (spec §10/§11). MVP scanning is signature/MIME based; ClamAV plugs in here.
func (w *Worker) handleScan(ctx context.Context, fileID string) {
	f, err := w.db.FileByIDAny(ctx, fileID)
	if err != nil {
		return
	}
	if f.Status == "deleted" || f.Status == "expired" {
		return
	}
	// 1. Declared MIME must be on the allowlist (spec §13/§15).
	verdict := scan(f)
	if !verdict.ok {
		w.reject(ctx, f, verdict.reason)
		return
	}
	// 2. The declared MIME must match the actual bytes (spec §15 MIME sniffing).
	// This blocks a renamed executable/script masquerading as a viewable type.
	switch w.sniffVerdict(ctx, f) {
	case sniffRetry:
		_ = w.rdb.Enqueue(ctx, "scan", fileID) // transient read error; retry later
		return
	case sniffReject:
		w.reject(ctx, f, "declared content type does not match file contents")
		return
	}
	_ = w.db.WriteAudit(ctx, audit.Entry{TenantID: f.TenantID, InstanceID: f.InstanceID,
		Event: audit.FileScanned, ActorType: audit.ActorSystem, ResourceType: "file", ResourceID: fileID})

	// APAGE is view-only: supported preview types (PDF/image/text) become ready
	// directly. Office documents are not converted or editable; they are simply
	// not an accepted upload type (rejected at the MIME allowlist).
	_ = w.db.SetFileStatus(ctx, fileID, "ready", "ready", "")
	w.log.Info("file ready", "file", fileID, "mime", f.MimeType)
}

// handleDelete removes objects, retrying via re-enqueue on failure (spec §11
// tombstone + retry).
func (w *Worker) handleDelete(ctx context.Context, key string) {
	if w.obj == nil {
		return
	}
	// Delete original + known derivatives (spec §11).
	base := strings.TrimSuffix(key, "/original")
	keys := []string{key, base + "/preview.pdf", base + "/thumb.webp"}
	if err := w.obj.Delete(keys...); err != nil {
		w.log.Warn("object delete failed, re-queueing", "key", key, "err", err)
		_ = w.rdb.Enqueue(ctx, "delete", key) // retry (spec §11)
	}
}

// sweepLoop expires due files at least hourly (spec §11, SLO §18 P95<=2h).
func (w *Worker) sweepLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	w.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.sweep(ctx)
		}
	}
}

func (w *Worker) sweep(ctx context.Context) {
	due, err := w.db.ExpireDueFiles(ctx, time.Now())
	if err != nil {
		w.log.Warn("expire sweep", "err", err)
		return
	}
	for _, f := range due {
		_ = w.db.AddStorageUsed(ctx, f.TenantID, -f.Size)
		if f.StorageKey != "" {
			_ = w.rdb.Enqueue(ctx, "delete", f.StorageKey)
		}
		_ = w.db.WriteAudit(ctx, audit.Entry{TenantID: f.TenantID, InstanceID: f.InstanceID,
			Event: audit.FileExpired, ActorType: audit.ActorSystem, ResourceType: "file", ResourceID: f.FileID})
	}
	if len(due) > 0 {
		w.log.Info("expired files", "count", len(due))
	}
}

type verdict struct {
	ok     bool
	reason string
}

// reject moves a file to the rejected state with an audit reason (spec §10/§15).
func (w *Worker) reject(ctx context.Context, f *store.File, reason string) {
	_ = w.db.SetFileStatus(ctx, f.FileID, "rejected", "failed", reason)
	_ = w.db.WriteAudit(ctx, audit.Entry{TenantID: f.TenantID, InstanceID: f.InstanceID,
		Event: audit.FileRejected, ActorType: audit.ActorSystem, ResourceType: "file", ResourceID: f.FileID, Reason: reason})
}

type sniffResult int

const (
	sniffOK sniffResult = iota
	sniffReject
	sniffRetry
)

// sniffVerdict reads the leading bytes of the stored object and checks that the
// real content matches the declared MIME type. Fails closed on a content
// mismatch and retries on a transient read error (spec §15 MIME sniffing).
func (w *Worker) sniffVerdict(ctx context.Context, f *store.File) sniffResult {
	if w.obj == nil || f.StorageKey == "" {
		return sniffOK // nothing to read (e.g. storage disabled in tests)
	}
	body, _, _, err := w.obj.Get(f.StorageKey)
	if err != nil {
		w.log.Warn("scan: object read failed, will retry", "file", f.FileID, "err", err)
		return sniffRetry
	}
	defer body.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(body, head) // short reads return n<512 with head[:n] valid
	if sniffConsistent(f.MimeType, head[:n]) {
		return sniffOK
	}
	return sniffReject
}

// sniffConsistent reports whether the sniffed content type is compatible with
// the declared MIME family. Text-family types (json/csv/markdown/plain) all
// sniff as text/plain, so they are checked by family rather than exact match.
func sniffConsistent(declared string, head []byte) bool {
	sniffed := strings.SplitN(http.DetectContentType(head), ";", 2)[0]
	declared = strings.SplitN(strings.ToLower(strings.TrimSpace(declared)), ";", 2)[0]
	switch {
	case declared == "application/pdf":
		return sniffed == "application/pdf"
	case strings.HasPrefix(declared, "image/"):
		return strings.HasPrefix(sniffed, "image/")
	case declared == "text/html":
		return sniffed == "text/html" || sniffed == "text/plain"
	case strings.HasPrefix(declared, "text/"), declared == "application/json":
		return strings.HasPrefix(sniffed, "text/")
	}
	return false
}

// scan is the MVP scanner (spec §10). Replace with ClamAV/Safe Browsing in V1.
func scan(f *store.File) verdict {
	allowed := map[string]bool{
		"application/pdf": true, "image/png": true, "image/jpeg": true,
		"image/webp": true, "image/gif": true, "text/plain": true,
		"text/markdown": true, "application/json": true, "text/csv": true, "text/html": true,
	}
	if !allowed[f.MimeType] {
		return verdict{false, "mime type not allowed: " + f.MimeType}
	}
	return verdict{true, ""}
}

