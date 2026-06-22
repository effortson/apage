package store

import (
	"context"
	"errors"

	"github.com/apage/apage/internal/httpx"
	"github.com/jackc/pgx/v5"
)

// PlatformAdmin is a platform administrator identity (spec §8).
type PlatformAdmin struct {
	AdminID      string
	Email        string
	PasswordHash string
	TOTPSecret   string
	MFAEnrolled  bool
}

// CountPlatformAdmins returns the number of platform admins (bootstrap guard).
func (s *Store) CountPlatformAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM platform_admins`).Scan(&n)
	return n, err
}

// CreatePlatformAdmin inserts a platform admin (password set; MFA not yet enrolled).
func (s *Store) CreatePlatformAdmin(ctx context.Context, a PlatformAdmin) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO platform_admins(admin_id,email,password_hash) VALUES($1,$2,$3)`,
		a.AdminID, a.Email, a.PasswordHash)
	return err
}

func scanAdmin(row pgx.Row) (*PlatformAdmin, error) {
	var a PlatformAdmin
	var secret *string
	err := row.Scan(&a.AdminID, &a.Email, &a.PasswordHash, &secret, &a.MFAEnrolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if secret != nil {
		a.TOTPSecret = *secret
	}
	return &a, nil
}

const adminSelect = `SELECT admin_id,email,password_hash,totp_secret,mfa_enrolled FROM platform_admins`

// PlatformAdminByEmail loads an admin by email.
func (s *Store) PlatformAdminByEmail(ctx context.Context, email string) (*PlatformAdmin, error) {
	return scanAdmin(s.Pool.QueryRow(ctx, adminSelect+` WHERE email=$1`, email))
}

// PlatformAdminByID loads an admin by id.
func (s *Store) PlatformAdminByID(ctx context.Context, adminID string) (*PlatformAdmin, error) {
	return scanAdmin(s.Pool.QueryRow(ctx, adminSelect+` WHERE admin_id=$1`, adminID))
}

// SetAdminTOTP stores a (pending) TOTP secret during enrollment.
func (s *Store) SetAdminTOTP(ctx context.Context, adminID, secret string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE platform_admins SET totp_secret=$2 WHERE admin_id=$1`, adminID, secret)
	return err
}

// EnrollAdminMFA marks MFA as confirmed after the first valid code.
func (s *Store) EnrollAdminMFA(ctx context.Context, adminID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE platform_admins SET mfa_enrolled=true WHERE admin_id=$1`, adminID)
	return err
}

// TouchAdminLogin records a successful login time.
func (s *Store) TouchAdminLogin(ctx context.Context, adminID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE platform_admins SET last_login_at=now() WHERE admin_id=$1`, adminID)
	return err
}

// --- Tenant administration (metadata only, spec §8.2) ---

// CountTenants returns the total number of tenants (overview metric).
func (s *Store) CountTenants(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM tenants`).Scan(&n)
	return n, err
}

// ListTenantsAdmin lists tenants across the platform with cursor pagination and
// an optional name/id search (spec §8.2). Metadata only — no secrets.
func (s *Store) ListTenantsAdmin(ctx context.Context, p httpx.Page, search string) ([]Tenant, error) {
	q := `SELECT tenant_id,name,plan,trust_level,status,created_at FROM tenants WHERE 1=1`
	args := []any{}
	if search != "" {
		args = append(args, "%"+search+"%")
		q += ` AND (name ILIKE $` + itoa(len(args)) + ` OR tenant_id ILIKE $` + itoa(len(args)) + `)`
	}
	if p.Cursor != nil {
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
		q += ` AND (created_at,tenant_id) < ($` + itoa(len(args)-1) + `,$` + itoa(len(args)) + `)`
	}
	args = append(args, p.Limit)
	q += ` ORDER BY created_at DESC, tenant_id DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.TenantID, &t.Name, &t.Plan, &t.TrustLevel, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetTenantTrust updates a tenant's trust level (spec §8.2/§15.5).
func (s *Store) SetTenantTrust(ctx context.Context, tenantID, trust string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE tenants SET trust_level=$2 WHERE tenant_id=$1`, tenantID, trust)
	return tag.RowsAffected() > 0, err
}

// SetTenantStatus suspends or restores a tenant (spec §8.2/§15.5).
func (s *Store) SetTenantStatus(ctx context.Context, tenantID, status string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE tenants SET status=$2 WHERE tenant_id=$1`, tenantID, status)
	return tag.RowsAffected() > 0, err
}

// FreezeTenantLinks freezes all of a tenant's live links so a suspension takes
// immediate effect via the existing frozen_at runtime gate (spec §15.5).
func (s *Store) FreezeTenantLinks(ctx context.Context, tenantID, reason string) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=now(), frozen_reason=$2 WHERE tenant_id=$1 AND frozen_at IS NULL`,
		tenantID, reason)
	return tag.RowsAffected(), err
}

// UnfreezeTenantLinks lifts a suspension-induced freeze (spec §15.5).
func (s *Store) UnfreezeTenantLinks(ctx context.Context, tenantID, reason string) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE preview_links SET frozen_at=NULL, frozen_reason=NULL
		 WHERE tenant_id=$1 AND frozen_at IS NOT NULL AND frozen_reason=$2`, tenantID, reason)
	return tag.RowsAffected(), err
}

// TenantLinkIDs returns a tenant's link ids (for cache invalidation on suspend).
func (s *Store) TenantLinkIDs(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT link_id FROM preview_links WHERE tenant_id=$1`, tenantID)
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

// ActionAbuseReport sets an abuse report's status (open|actioned|dismissed, §8.3).
func (s *Store) ActionAbuseReport(ctx context.Context, reportID, status string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE abuse_reports SET status=$2 WHERE report_id=$1`, reportID, status)
	return tag.RowsAffected() > 0, err
}

// ListAuditAll returns audit logs across all tenants with cursor pagination and
// optional event/tenant filters (admin global audit, spec §8.5).
func (s *Store) ListAuditAll(ctx context.Context, p httpx.Page, event, tenantID string) ([]AuditLog, error) {
	q := auditSelect + ` WHERE 1=1`
	args := []any{}
	if event != "" {
		args = append(args, event)
		q += ` AND event=$` + itoa(len(args))
	}
	if tenantID != "" {
		args = append(args, tenantID)
		q += ` AND tenant_id=$` + itoa(len(args))
	}
	if p.Cursor != nil {
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
		q += ` AND (created_at,event_id) < ($` + itoa(len(args)-1) + `,$` + itoa(len(args)) + `)`
	}
	args = append(args, p.Limit)
	q += ` ORDER BY created_at DESC, event_id DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditLog
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.EventID, &a.TenantID, &a.InstanceID, &a.Event, &a.ActorType,
			&a.ActorID, &a.ResourceType, &a.ResourceID, &a.IP, &a.UserAgent, &a.Reason, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
