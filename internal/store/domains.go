package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CreateDomain adds a custom domain, enforcing custom_domain_limit (spec §28).
func (s *Store) CreateDomain(ctx context.Context, d CustomDomain) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var used, limit int
	if err := tx.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM custom_domains WHERE tenant_id=$1),
		        (SELECT custom_domain_limit FROM quotas WHERE tenant_id=$1)`, d.TenantID).
		Scan(&used, &limit); err != nil {
		return err
	}
	if used >= limit {
		return ErrQuotaExceeded
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO custom_domains(domain_id,tenant_id,domain,status,txt_value) VALUES($1,$2,$3,'pending',$4)`,
		d.DomainID, d.TenantID, d.Domain, d.TXTValue)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const domainSelect = `SELECT domain_id,tenant_id,domain,status,txt_value,cert_status,last_checked_at,created_at FROM custom_domains`

// ListDomains returns a tenant's custom domains (spec §28).
func (s *Store) ListDomains(ctx context.Context, tenantID string) ([]CustomDomain, error) {
	rows, err := s.Pool.Query(ctx, domainSelect+` WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDomain
	for rows.Next() {
		var d CustomDomain
		if err := rows.Scan(&d.DomainID, &d.TenantID, &d.Domain, &d.Status, &d.TXTValue,
			&d.CertStatus, &d.LastCheckedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DomainByID loads a custom domain scoped to a tenant.
func (s *Store) DomainByID(ctx context.Context, tenantID, domainID string) (*CustomDomain, error) {
	var d CustomDomain
	err := s.Pool.QueryRow(ctx, domainSelect+` WHERE domain_id=$1 AND tenant_id=$2`, domainID, tenantID).
		Scan(&d.DomainID, &d.TenantID, &d.Domain, &d.Status, &d.TXTValue, &d.CertStatus, &d.LastCheckedAt, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

// SetDomainStatus updates verification/cert status after a check (spec §28).
func (s *Store) SetDomainStatus(ctx context.Context, domainID, status, certStatus string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE custom_domains SET status=$2, cert_status=$3, last_checked_at=now() WHERE domain_id=$1`,
		domainID, status, certStatus)
	return err
}

// DeleteDomain removes a custom domain.
func (s *Store) DeleteDomain(ctx context.Context, tenantID, domainID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM custom_domains WHERE domain_id=$1 AND tenant_id=$2`, domainID, tenantID)
	return err
}
