package store

import "context"

// PurgeResult reports what a data-deletion request removed (spec §15.6).
type PurgeResult struct {
	Files       int
	Links       int
	StorageKeys []string // object keys to delete from storage
}

// PurgeTenantData deletes a tenant's content per a GDPR/CCPA deletion request
// (spec §15.6): cloud files and preview links. The tenant/users/audit records
// are retained; object deletion is returned to the caller to enqueue with
// tombstone+retry (spec §11).
func (s *Store) PurgeTenantData(ctx context.Context, tenantID string) (*PurgeResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	res := &PurgeResult{}

	// Collect storage keys before deleting file rows.
	rows, err := tx.Query(ctx, `SELECT storage_key FROM files WHERE tenant_id=$1 AND storage_key IS NOT NULL AND storage_key<>''`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return nil, err
		}
		res.StorageKeys = append(res.StorageKeys, k)
	}
	rows.Close()

	ct, err := tx.Exec(ctx, `DELETE FROM preview_links WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return nil, err
	}
	res.Links = int(ct.RowsAffected())

	ct, err = tx.Exec(ctx, `DELETE FROM files WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return nil, err
	}
	res.Files = int(ct.RowsAffected())

	if _, err := tx.Exec(ctx, `UPDATE quotas SET storage_bytes_used=0 WHERE tenant_id=$1`, tenantID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}
