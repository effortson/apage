package store

import (
	"context"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
)

// WriteAudit persists an audit entry (spec §15). Callers typically enqueue this
// asynchronously; for MVP single-box it writes directly.
func (s *Store) WriteAudit(ctx context.Context, e audit.Entry) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO audit_logs(event_id,tenant_id,instance_id,event,actor_type,actor_id,
			resource_type,resource_id,ip,user_agent,reason)
		 VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),
			NULLIF($9,''),NULLIF($10,''),NULLIF($11,''))`,
		id.New(id.PrefixAudit), e.TenantID, e.InstanceID, e.Event, e.ActorType, e.ActorID,
		e.ResourceType, e.ResourceID, e.IP, e.UserAgent, e.Reason)
	return err
}

const auditSelect = `SELECT event_id,COALESCE(tenant_id,''),COALESCE(instance_id,''),event,actor_type,
	COALESCE(actor_id,''),COALESCE(resource_type,''),COALESCE(resource_id,''),COALESCE(ip,''),
	COALESCE(user_agent,''),COALESCE(reason,''),created_at FROM audit_logs`

// ListAudit returns audit logs with filters + cursor pagination (spec §14).
func (s *Store) ListAudit(ctx context.Context, tenantID string, p httpx.Page, event, resourceType, resourceID, actorType string) ([]AuditLog, error) {
	q := auditSelect + ` WHERE tenant_id=$1`
	args := []any{tenantID}
	if event != "" {
		args = append(args, event)
		q += ` AND event=$` + itoa(len(args))
	}
	if resourceType != "" {
		args = append(args, resourceType)
		q += ` AND resource_type=$` + itoa(len(args))
	}
	if resourceID != "" {
		args = append(args, resourceID)
		q += ` AND resource_id=$` + itoa(len(args))
	}
	if actorType != "" {
		args = append(args, actorType)
		q += ` AND actor_type=$` + itoa(len(args))
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

// --- Abuse reports (spec §15.5, §30) ---

// CreateAbuseReport stores a public abuse report.
func (s *Store) CreateAbuseReport(ctx context.Context, r AbuseReport) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO abuse_reports(report_id,link_id,tenant_id,category,detail,source_ip)
		 VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,$5,$6)`,
		r.ReportID, r.LinkID, r.TenantID, r.Category, r.Detail, r.SourceIP)
	return err
}

// ListAbuseReports lists open abuse reports (admin queue, spec §8.3).
func (s *Store) ListAbuseReports(ctx context.Context, p httpx.Page, status string) ([]AbuseReport, error) {
	q := `SELECT report_id,COALESCE(link_id,''),COALESCE(tenant_id,''),category,COALESCE(detail,''),
		status,created_at FROM abuse_reports WHERE 1=1`
	args := []any{}
	if status != "" {
		args = append(args, status)
		q += ` AND status=$` + itoa(len(args))
	}
	if p.Cursor != nil {
		args = append(args, p.Cursor.CreatedAt, p.Cursor.ID)
		q += ` AND (created_at,report_id) < ($` + itoa(len(args)-1) + `,$` + itoa(len(args)) + `)`
	}
	args = append(args, p.Limit)
	q += ` ORDER BY created_at DESC, report_id DESC LIMIT $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AbuseReport
	for rows.Next() {
		var r AbuseReport
		if err := rows.Scan(&r.ReportID, &r.LinkID, &r.TenantID, &r.Category, &r.Detail, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
