package store

import (
	"context"
	"errors"

	"github.com/apage/apage/internal/httpx"
	"github.com/jackc/pgx/v5"
)

// CreateInstance inserts an instance with hashed credentials (spec §26).
// Enforces instance_limit inside the transaction.
func (s *Store) CreateInstance(ctx context.Context, in Instance, tokenHash, keyHash string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var used, limit int
	if err := tx.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM agent_instances WHERE tenant_id=$1),
		        (SELECT instance_limit FROM quotas WHERE tenant_id=$1)`,
		in.TenantID).Scan(&used, &limit); err != nil {
		return err
	}
	if used >= limit {
		return ErrQuotaExceeded
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO agent_instances(instance_id,tenant_id,agent_type,agent_name,subdomain,mode,
			agent_token_hash,instance_api_key_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		in.InstanceID, in.TenantID, in.AgentType, in.AgentName, in.Subdomain, in.Mode, tokenHash, keyHash)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ErrQuotaExceeded indicates a plan limit was hit (spec §2).
var ErrQuotaExceeded = errors.New("quota exceeded")

// ErrSubdomainTaken indicates a subdomain collision.
var ErrSubdomainTaken = errors.New("subdomain taken")

// InstanceByID loads an instance (no credentials).
func (s *Store) InstanceByID(ctx context.Context, instanceID string) (*Instance, error) {
	return s.scanInstance(s.Pool.QueryRow(ctx, instanceSelect+` WHERE instance_id=$1`, instanceID))
}

// InstanceBySubdomain resolves a preview subdomain to its instance (runtime).
func (s *Store) InstanceBySubdomain(ctx context.Context, subdomain string) (*Instance, error) {
	return s.scanInstance(s.Pool.QueryRow(ctx, instanceSelect+` WHERE subdomain=$1`, subdomain))
}

const instanceSelect = `SELECT instance_id,tenant_id,agent_type,agent_name,subdomain,mode,status,
	COALESCE(agent_version,''),last_seen_at,frozen_at,created_at FROM agent_instances`

func (s *Store) scanInstance(row pgx.Row) (*Instance, error) {
	var in Instance
	err := row.Scan(&in.InstanceID, &in.TenantID, &in.AgentType, &in.AgentName, &in.Subdomain,
		&in.Mode, &in.Status, &in.AgentVersion, &in.LastSeenAt, &in.FrozenAt, &in.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &in, err
}

// ListInstances returns a tenant's instances with cursor pagination (spec §26).
func (s *Store) ListInstances(ctx context.Context, tenantID string, p httpx.Page) ([]Instance, error) {
	q := instanceSelect + ` WHERE tenant_id=$1`
	args := []any{tenantID}
	if p.Cursor != nil {
		q += ` AND (created_at,instance_id) < ($2,$3)`
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
	}
	q += ` ORDER BY created_at DESC, instance_id DESC LIMIT $` + itoa(len(args)+1)
	args = append(args, p.Limit)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Instance
	for rows.Next() {
		in, err := s.scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// VerifyInstanceAPIKey resolves an instance by api-key hash (data-plane auth).
// Accepts the active key or a grace-period rotated key (spec §凭证生命周期).
func (s *Store) VerifyInstanceAPIKey(ctx context.Context, keyHash string) (*Instance, error) {
	return s.scanInstance(s.Pool.QueryRow(ctx,
		instanceSelect+` WHERE instance_api_key_hash=$1 OR token_grace_hash=$1`, keyHash))
}

// VerifyAgentToken resolves an instance by agent-token hash (tunnel auth).
func (s *Store) VerifyAgentToken(ctx context.Context, tokenHash string) (*Instance, error) {
	return s.scanInstance(s.Pool.QueryRow(ctx,
		instanceSelect+` WHERE agent_token_hash=$1`, tokenHash))
}

// SetInstanceStatus updates status + last_seen + version on connect/disconnect.
func (s *Store) SetInstanceStatus(ctx context.Context, instanceID, status, version string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE agent_instances SET status=$2, last_seen_at=now(),
		   agent_version=COALESCE(NULLIF($3,''),agent_version) WHERE instance_id=$1`,
		instanceID, status, version)
	return err
}

// RotateCredentials installs new credential hashes, keeping the old api key as a
// grace-period key (spec §凭证生命周期 / §26).
func (s *Store) RotateCredentials(ctx context.Context, instanceID, newTokenHash, newKeyHash string) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE agent_instances
		 SET token_grace_hash = instance_api_key_hash,
		     agent_token_hash=$2, instance_api_key_hash=$3 WHERE instance_id=$1`,
		instanceID, newTokenHash, newKeyHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAgentToken invalidates the agent token immediately (spec §26).
func (s *Store) RevokeAgentToken(ctx context.Context, instanceID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE agent_instances SET agent_token_hash='revoked:'||instance_id WHERE instance_id=$1`, instanceID)
	return err
}

// DeleteInstance removes an instance (cascades links via FK).
func (s *Store) DeleteInstance(ctx context.Context, instanceID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM agent_instances WHERE instance_id=$1`, instanceID)
	return err
}

// FreezeInstance marks an instance frozen (abuse governance, spec §15.5).
// Tenant-scoped; the bool reports whether a matching instance was affected.
func (s *Store) FreezeInstance(ctx context.Context, tenantID, instanceID string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE agent_instances SET frozen_at=now(), status='offline'
		 WHERE instance_id=$1 AND tenant_id=$2 AND frozen_at IS NULL`, instanceID, tenantID)
	return tag.RowsAffected() > 0, err
}

// UnfreezeInstance lifts an instance freeze (spec §15.5).
func (s *Store) UnfreezeInstance(ctx context.Context, tenantID, instanceID string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE agent_instances SET frozen_at=NULL
		 WHERE instance_id=$1 AND tenant_id=$2 AND frozen_at IS NOT NULL`, instanceID, tenantID)
	return tag.RowsAffected() > 0, err
}

// FreezeInstanceLinks freezes all of an instance's live links so an instance
// freeze takes immediate effect via the existing frozen_at runtime gate. The
// reason tags these so unfreezing only lifts the instance-induced freeze and
// leaves independent abuse freezes intact (security review #1).
func (s *Store) FreezeInstanceLinks(ctx context.Context, tenantID, instanceID, reason string) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=now(), frozen_reason=$3
		 WHERE tenant_id=$1 AND instance_id=$2 AND frozen_at IS NULL`,
		tenantID, instanceID, reason)
	return tag.RowsAffected(), err
}

// UnfreezeInstanceLinks lifts only the instance-induced link freeze.
func (s *Store) UnfreezeInstanceLinks(ctx context.Context, tenantID, instanceID, reason string) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=NULL, frozen_reason=NULL
		 WHERE tenant_id=$1 AND instance_id=$2 AND frozen_at IS NOT NULL AND frozen_reason=$3`,
		tenantID, instanceID, reason)
	return tag.RowsAffected(), err
}

// InstanceLinkIDs returns an instance's link ids (for cache invalidation).
func (s *Store) InstanceLinkIDs(ctx context.Context, instanceID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT link_id FROM preview_links WHERE instance_id=$1`, instanceID)
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
