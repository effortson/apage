package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a row does not exist (mapped to 404 by handlers).
var ErrNotFound = errors.New("not found")

// Plan limit presets (spec §20).
var planQuotas = map[string]Quota{
	"lite":    {InstanceLimit: 1, StorageBytesLimit: 100 << 20, TunnelEgressLimit: 1 << 30, CloudEgressLimit: 1 << 30, CustomDomainLimit: 0},
	"starter": {InstanceLimit: 1, StorageBytesLimit: 1 << 30, TunnelEgressLimit: 10 << 30, CloudEgressLimit: 10 << 30, CustomDomainLimit: 1},
	"pro":     {InstanceLimit: 5, StorageBytesLimit: 50 << 30, TunnelEgressLimit: 100 << 30, CloudEgressLimit: 100 << 30, CustomDomainLimit: 5},
	"team":    {InstanceLimit: 25, StorageBytesLimit: 500 << 30, TunnelEgressLimit: 1000 << 30, CloudEgressLimit: 1000 << 30, CustomDomainLimit: 25},
}

// PlanQuota returns the limit preset for a plan (defaults to lite).
func PlanQuota(plan string) Quota {
	if q, ok := planQuotas[plan]; ok {
		return q
	}
	return planQuotas["lite"]
}

// RegisterAccount atomically creates a tenant, user, owner membership, and quota
// (spec §25 register). All-or-nothing in one transaction.
func (s *Store) RegisterAccount(ctx context.Context, tenant Tenant, user User, membershipID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO tenants(tenant_id,name,plan,trust_level) VALUES($1,$2,$3,$4)`,
		tenant.TenantID, tenant.Name, tenant.Plan, tenant.TrustLevel); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO users(user_id,email,auth_provider,password_hash) VALUES($1,$2,$3,$4)`,
		user.UserID, user.Email, user.AuthProvider, user.PasswordHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO memberships(membership_id,user_id,tenant_id,role) VALUES($1,$2,$3,'owner')`,
		membershipID, user.UserID, tenant.TenantID); err != nil {
		return err
	}
	q := PlanQuota(tenant.Plan)
	if _, err := tx.Exec(ctx,
		`INSERT INTO quotas(tenant_id,plan,instance_limit,storage_bytes_limit,tunnel_egress_limit,
			cloud_egress_limit,custom_domain_limit)
		 VALUES($1,$2,$3,$4,$5,$6,$7)`,
		tenant.TenantID, tenant.Plan, q.InstanceLimit, q.StorageBytesLimit, q.TunnelEgressLimit,
		q.CloudEgressLimit, q.CustomDomainLimit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UserByEmail loads a user by email.
func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`SELECT user_id,email,email_verified_at,auth_provider,COALESCE(password_hash,''),created_at
		 FROM users WHERE email=$1`, email).
		Scan(&u.UserID, &u.Email, &u.EmailVerifiedAt, &u.AuthProvider, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// UserByID loads a user by id.
func (s *Store) UserByID(ctx context.Context, userID string) (*User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`SELECT user_id,email,email_verified_at,auth_provider,COALESCE(password_hash,''),created_at
		 FROM users WHERE user_id=$1`, userID).
		Scan(&u.UserID, &u.Email, &u.EmailVerifiedAt, &u.AuthProvider, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// MarkEmailVerified sets email_verified_at = now.
func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET email_verified_at=now() WHERE user_id=$1`, userID)
	return err
}

// SetPassword updates a user's password hash.
func (s *Store) SetPassword(ctx context.Context, userID, hash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE user_id=$1`, userID, hash)
	return err
}

// TenantByID loads a tenant.
func (s *Store) TenantByID(ctx context.Context, tenantID string) (*Tenant, error) {
	var t Tenant
	err := s.Pool.QueryRow(ctx,
		`SELECT tenant_id,name,plan,trust_level,status,created_at FROM tenants WHERE tenant_id=$1`, tenantID).
		Scan(&t.TenantID, &t.Name, &t.Plan, &t.TrustLevel, &t.Status, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

// MembershipsForUser returns all tenants a user belongs to.
func (s *Store) MembershipsForUser(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT membership_id,user_id,tenant_id,role,created_at FROM memberships WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.MembershipID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MembershipFor returns the membership of a user within a tenant (RBAC lookup).
func (s *Store) MembershipFor(ctx context.Context, userID, tenantID string) (*Membership, error) {
	var m Membership
	err := s.Pool.QueryRow(ctx,
		`SELECT membership_id,user_id,tenant_id,role,created_at FROM memberships WHERE user_id=$1 AND tenant_id=$2`,
		userID, tenantID).Scan(&m.MembershipID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

// ListMembers lists members of a tenant with emails (spec §27).
func (s *Store) ListMembers(ctx context.Context, tenantID string) ([]Membership, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT m.membership_id,m.user_id,m.tenant_id,m.role,m.created_at,u.email
		 FROM memberships m JOIN users u ON u.user_id=m.user_id
		 WHERE m.tenant_id=$1 ORDER BY m.created_at ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.MembershipID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt, &m.Email); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMembership inserts a membership (invite accept, spec §27).
func (s *Store) CreateMembership(ctx context.Context, m Membership) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO memberships(membership_id,user_id,tenant_id,role) VALUES($1,$2,$3,$4)
		 ON CONFLICT (user_id,tenant_id) DO NOTHING`,
		m.MembershipID, m.UserID, m.TenantID, m.Role)
	return err
}

// CountOwners returns the number of owners in a tenant (spec §27: keep >=1 owner).
func (s *Store) CountOwners(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE tenant_id=$1 AND role='owner'`, tenantID).Scan(&n)
	return n, err
}

// UpdateMemberRole changes a membership role.
func (s *Store) UpdateMemberRole(ctx context.Context, membershipID, role string) error {
	ct, err := s.Pool.Exec(ctx, `UPDATE memberships SET role=$2 WHERE membership_id=$1`, membershipID, role)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MembershipByID loads a membership.
func (s *Store) MembershipByID(ctx context.Context, membershipID string) (*Membership, error) {
	var m Membership
	err := s.Pool.QueryRow(ctx,
		`SELECT membership_id,user_id,tenant_id,role,created_at FROM memberships WHERE membership_id=$1`, membershipID).
		Scan(&m.MembershipID, &m.UserID, &m.TenantID, &m.Role, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

// DeleteMembership removes a membership.
func (s *Store) DeleteMembership(ctx context.Context, membershipID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM memberships WHERE membership_id=$1`, membershipID)
	return err
}

// QuotaFor loads a tenant's quota/usage (spec §2, §29).
func (s *Store) QuotaFor(ctx context.Context, tenantID string) (*Quota, error) {
	var q Quota
	err := s.Pool.QueryRow(ctx,
		`SELECT tenant_id,plan,instance_limit,storage_bytes_limit,storage_bytes_used,
			tunnel_egress_limit,tunnel_egress_used,cloud_egress_limit,cloud_egress_used,
			custom_domain_limit,custom_domain_used,period_start
		 FROM quotas WHERE tenant_id=$1`, tenantID).
		Scan(&q.TenantID, &q.Plan, &q.InstanceLimit, &q.StorageBytesLimit, &q.StorageBytesUsed,
			&q.TunnelEgressLimit, &q.TunnelEgressUsed, &q.CloudEgressLimit, &q.CloudEgressUsed,
			&q.CustomDomainLimit, &q.CustomDomainUsed, &q.PeriodStart)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &q, err
}

// AddStorageUsed adjusts storage usage (delta may be negative).
// UpdateTenantName renames a tenant (settings profile, UI §7.9).
func (s *Store) UpdateTenantName(ctx context.Context, tenantID, name string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE tenants SET name=$2 WHERE tenant_id=$1`, tenantID, name)
	return err
}

func (s *Store) AddStorageUsed(ctx context.Context, tenantID string, delta int64) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE quotas SET storage_bytes_used = GREATEST(0, storage_bytes_used + $2) WHERE tenant_id=$1`,
		tenantID, delta)
	return err
}

// --- Sessions (spec §25) ---

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, sessionID, userID string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO sessions(session_id,user_id,expires_at) VALUES($1,$2,$3)`, sessionID, userID, expiresAt)
	return err
}

// SessionUser returns the user id for a valid (unexpired) session.
func (s *Store) SessionUser(ctx context.Context, sessionID string) (string, error) {
	var userID string
	err := s.Pool.QueryRow(ctx,
		`SELECT user_id FROM sessions WHERE session_id=$1 AND expires_at > now()`, sessionID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE session_id=$1`, sessionID)
	return err
}

// --- Auth tokens (verify email / reset / invite) ---

// CreateAuthToken stores a hashed auth token.
func (s *Store) CreateAuthToken(ctx context.Context, tokenHash, userID, tenantID, purpose, email, role string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO auth_tokens(token_hash,user_id,tenant_id,purpose,email,role,expires_at)
		 VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),$7)`,
		tokenHash, userID, tenantID, purpose, email, role, expiresAt)
	return err
}

// AuthTokenRow is a consumed auth token.
type AuthTokenRow struct {
	UserID   string
	TenantID string
	Purpose  string
	Email    string
	Role     string
}

// ConsumeAuthToken atomically fetches and deletes a valid token.
func (s *Store) ConsumeAuthToken(ctx context.Context, tokenHash, purpose string) (*AuthTokenRow, error) {
	var r AuthTokenRow
	err := s.Pool.QueryRow(ctx,
		`DELETE FROM auth_tokens WHERE token_hash=$1 AND purpose=$2 AND expires_at>now()
		 RETURNING COALESCE(user_id,''),COALESCE(tenant_id,''),purpose,COALESCE(email,''),COALESCE(role,'')`,
		tokenHash, purpose).Scan(&r.UserID, &r.TenantID, &r.Purpose, &r.Email, &r.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}
