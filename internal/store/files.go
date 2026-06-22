package store

import (
	"context"
	"errors"
	"time"

	"github.com/apage/apage/internal/httpx"
	"github.com/jackc/pgx/v5"
)

// --- File refs (tunnel, spec §2) ---

// UpsertFileRef stores a tunnel file ref reported by an agent (spec §6.4).
func (s *Store) UpsertFileRef(ctx context.Context, f FileRef) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO file_refs(file_ref,instance_id,display_name,size,mime_type,modified_at,expires_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (file_ref) DO UPDATE SET display_name=EXCLUDED.display_name,
		   size=EXCLUDED.size, mime_type=EXCLUDED.mime_type, expires_at=EXCLUDED.expires_at`,
		f.FileRef, f.InstanceID, f.DisplayName, f.Size, f.MimeType, f.ModifiedAt, f.ExpiresAt)
	return err
}

// FileRefByID loads a file ref.
func (s *Store) FileRefByID(ctx context.Context, ref string) (*FileRef, error) {
	var f FileRef
	err := s.Pool.QueryRow(ctx,
		`SELECT file_ref,instance_id,display_name,size,COALESCE(mime_type,''),modified_at,expires_at,created_at
		 FROM file_refs WHERE file_ref=$1`, ref).
		Scan(&f.FileRef, &f.InstanceID, &f.DisplayName, &f.Size, &f.MimeType, &f.ModifiedAt, &f.ExpiresAt, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

// --- Cloud files (spec §11) ---

const fileSelect = `SELECT file_id,tenant_id,COALESCE(instance_id,''),status,preview_status,
	display_name,size,COALESCE(mime_type,''),COALESCE(storage_key,''),visibility,
	COALESCE(reject_reason,''),expires_at,created_at FROM files`

func scanFile(row pgx.Row) (*File, error) {
	var f File
	err := row.Scan(&f.FileID, &f.TenantID, &f.InstanceID, &f.Status, &f.PreviewStatus,
		&f.DisplayName, &f.Size, &f.MimeType, &f.StorageKey, &f.Visibility, &f.RejectReason,
		&f.ExpiresAt, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

// CreateFile inserts a file record (spec §11/§12). Initial status is never ready.
func (s *Store) CreateFile(ctx context.Context, f File) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO files(file_id,tenant_id,instance_id,status,preview_status,display_name,size,
			mime_type,storage_key,visibility,expires_at)
		 VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)`,
		f.FileID, f.TenantID, f.InstanceID, f.Status, f.PreviewStatus, f.DisplayName, f.Size,
		f.MimeType, f.StorageKey, f.Visibility, f.ExpiresAt)
	return err
}

// FileByID loads a file scoped to a tenant.
func (s *Store) FileByID(ctx context.Context, tenantID, fileID string) (*File, error) {
	return scanFile(s.Pool.QueryRow(ctx, fileSelect+` WHERE file_id=$1 AND tenant_id=$2`, fileID, tenantID))
}

// FileByIDAny loads a file without tenant scoping (runtime resolves by link).
func (s *Store) FileByIDAny(ctx context.Context, fileID string) (*File, error) {
	return scanFile(s.Pool.QueryRow(ctx, fileSelect+` WHERE file_id=$1`, fileID))
}

// SetFileStatus transitions a file's status (spec §11 state machine).
func (s *Store) SetFileStatus(ctx context.Context, fileID, status, previewStatus, rejectReason string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE files SET status=$2,
		   preview_status=COALESCE(NULLIF($3,''),preview_status),
		   reject_reason=NULLIF($4,'') WHERE file_id=$1`,
		fileID, status, previewStatus, rejectReason)
	return err
}

// FinalizeUpload marks an uploaded file ready for scanning (spec §12 complete).
func (s *Store) FinalizeUpload(ctx context.Context, fileID string, size int64) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE files SET status='scanning', size=$2 WHERE file_id=$1`, fileID, size)
	return err
}

// ListFiles returns a tenant's files with cursor pagination (spec §14).
func (s *Store) ListFiles(ctx context.Context, tenantID string, p httpx.Page, status, instanceID string) ([]File, error) {
	q := fileSelect + ` WHERE tenant_id=$1`
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		q += ` AND status=$` + itoa(len(args))
	}
	if instanceID != "" {
		args = append(args, instanceID)
		q += ` AND instance_id=$` + itoa(len(args))
	}
	if p.Cursor != nil {
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
		q += ` AND (created_at,file_id) < ($` + itoa(len(args)-1) + `,$` + itoa(len(args)) + `)`
	}
	args = append(args, p.Limit)
	q += ` ORDER BY created_at DESC, file_id DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// MarkFileDeleted sets status=deleted (spec §11). Object cleanup is async.
func (s *Store) MarkFileDeleted(ctx context.Context, tenantID, fileID string) (*File, error) {
	f, err := s.FileByID(ctx, tenantID, fileID)
	if err != nil {
		return nil, err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE files SET status='deleted' WHERE file_id=$1`, fileID)
	return f, err
}

// ExpireDueFiles flips files past their expiry to expired (worker, spec §11).
func (s *Store) ExpireDueFiles(ctx context.Context, now time.Time) ([]File, error) {
	rows, err := s.Pool.Query(ctx,
		fileSelect+` WHERE expires_at IS NOT NULL AND expires_at<=$1 AND status NOT IN('expired','deleted','rejected')`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		due = append(due, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, f := range due {
		_, _ = s.Pool.Exec(ctx, `UPDATE files SET status='expired' WHERE file_id=$1`, f.FileID)
	}
	return due, nil
}
