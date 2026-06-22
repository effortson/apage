package store

import (
	"context"
	"errors"
	"time"

	"github.com/apage/apage/internal/httpx"
	"github.com/jackc/pgx/v5"
)

const linkSelect = `SELECT link_id,tenant_id,COALESCE(instance_id,''),file_id,mode,
	COALESCE(display_name,''),secret_hash,access_policy,expires_at,revoked_at,frozen_at,
	COALESCE(frozen_reason,''),last_accessed_at,view_count,created_at FROM preview_links`

// linkRow is the full row including secret_hash (internal use only).
type linkRow struct {
	PreviewLink
	SecretHash string
}

func scanLink(row pgx.Row) (*linkRow, error) {
	var l linkRow
	err := row.Scan(&l.LinkID, &l.TenantID, &l.InstanceID, &l.FileID, &l.Mode,
		&l.DisplayName, &l.SecretHash, &l.AccessPolicy, &l.ExpiresAt, &l.RevokedAt, &l.FrozenAt,
		&l.FrozenReason, &l.LastAccessedAt, &l.ViewCount, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &l, err
}

// CreateLink inserts a preview link.
func (s *Store) CreateLink(ctx context.Context, l PreviewLink, secretHash string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO preview_links(link_id,tenant_id,instance_id,file_id,mode,display_name,
			secret_hash,access_policy,expires_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9)`,
		l.LinkID, l.TenantID, l.InstanceID, l.FileID, l.Mode, l.DisplayName,
		secretHash, l.AccessPolicy, l.ExpiresAt)
	return err
}

// LinkUpdate carries the optional fields an agent may change on an existing
// link via modify_link (spec: agent-driven link management). A nil field is left
// unchanged; FileID swaps the backing cloud file (keeps the same URL/secret).
type LinkUpdate struct {
	FileID       *string
	DisplayName  *string
	AccessPolicy []byte
	ExpiresAt    *time.Time
}

// UpdateLink applies a partial update to a tenant's link, leaving the secret and
// link id intact so the public URL keeps working. Returns ErrNotFound when no
// matching, non-revoked link exists.
func (s *Store) UpdateLink(ctx context.Context, tenantID, linkID string, u LinkUpdate) error {
	sets := []string{}
	args := []any{linkID, tenantID}
	if u.FileID != nil {
		args = append(args, *u.FileID)
		sets = append(sets, `file_id=$`+itoa(len(args)))
	}
	if u.DisplayName != nil {
		args = append(args, *u.DisplayName)
		sets = append(sets, `display_name=$`+itoa(len(args)))
	}
	if u.AccessPolicy != nil {
		args = append(args, u.AccessPolicy)
		sets = append(sets, `access_policy=$`+itoa(len(args)))
	}
	if u.ExpiresAt != nil {
		args = append(args, *u.ExpiresAt)
		sets = append(sets, `expires_at=$`+itoa(len(args)))
	}
	if len(sets) == 0 {
		return nil // nothing to change
	}
	q := `UPDATE preview_links SET ` + join(sets, ", ") +
		` WHERE link_id=$1 AND tenant_id=$2 AND revoked_at IS NULL`
	tag, err := s.Pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkByID loads a link with its secret hash (runtime access path).
func (s *Store) LinkByID(ctx context.Context, linkID string) (*linkRow, error) {
	return scanLink(s.Pool.QueryRow(ctx, linkSelect+` WHERE link_id=$1`, linkID))
}

// SecretHashOf exposes a link's stored secret hash for constant-time compare.
func (l *linkRow) SecretHashOf() string { return l.SecretHash }

// ListLinks returns a tenant's links with filters + cursor pagination (spec §14).
func (s *Store) ListLinks(ctx context.Context, tenantID string, p httpx.Page, status, mode, instanceID string) ([]PreviewLink, error) {
	q := linkSelect + ` WHERE tenant_id=$1`
	args := []any{tenantID}
	switch status {
	case "active":
		q += ` AND revoked_at IS NULL AND frozen_at IS NULL AND (expires_at IS NULL OR expires_at>now())`
	case "revoked":
		q += ` AND revoked_at IS NOT NULL`
	case "expired":
		q += ` AND expires_at IS NOT NULL AND expires_at<=now()`
	case "frozen":
		q += ` AND frozen_at IS NOT NULL`
	}
	if mode != "" {
		args = append(args, mode)
		q += ` AND mode=$` + itoa(len(args))
	}
	if instanceID != "" {
		args = append(args, instanceID)
		q += ` AND instance_id=$` + itoa(len(args))
	}
	if p.Cursor != nil {
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
		q += ` AND (created_at,link_id) < ($` + itoa(len(args)-1) + `,$` + itoa(len(args)) + `)`
	}
	args = append(args, p.Limit)
	q += ` ORDER BY created_at DESC, link_id DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PreviewLink
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l.PreviewLink) // secret hash dropped from list output
	}
	return out, rows.Err()
}

// RevokeLink sets revoked_at (spec §14).
func (s *Store) RevokeLink(ctx context.Context, tenantID, linkID string) (time.Time, error) {
	var t time.Time
	err := s.Pool.QueryRow(ctx,
		`UPDATE preview_links SET revoked_at=now() WHERE link_id=$1 AND tenant_id=$2 AND revoked_at IS NULL
		 RETURNING revoked_at`, linkID, tenantID).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	return t, err
}

// FreezeLink freezes a link for abuse review (spec §15.5). Tenant-scoped; the
// bool reports whether a matching, not-already-frozen link was affected.
func (s *Store) FreezeLink(ctx context.Context, tenantID, linkID, reason string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=now(), frozen_reason=$3
		 WHERE link_id=$1 AND tenant_id=$2 AND frozen_at IS NULL`, linkID, tenantID, reason)
	return tag.RowsAffected() > 0, err
}

// UnfreezeLink lifts an abuse freeze (appeal resolved, spec §15.5).
func (s *Store) UnfreezeLink(ctx context.Context, tenantID, linkID string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=NULL, frozen_reason=NULL
		 WHERE link_id=$1 AND tenant_id=$2 AND frozen_at IS NOT NULL`, linkID, tenantID)
	return tag.RowsAffected() > 0, err
}

// TouchLinkAccess flushes view stats asynchronously (spec §19.7 final-consistency).
func (s *Store) TouchLinkAccess(ctx context.Context, linkID string, viewCount int64) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET last_accessed_at=now(), view_count=$2 WHERE link_id=$1`, linkID, viewCount)
	return err
}

// LinksByFile returns links backing a cloud file (for cascade invalidation, spec §11).
func (s *Store) LinksByFile(ctx context.Context, fileID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT link_id FROM preview_links WHERE file_id=$1`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
