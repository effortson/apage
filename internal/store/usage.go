package store

import (
	"context"
	"fmt"
	"time"
)

// usageQuotaColumn maps a metering dimension to its cumulative quotas column.
var usageQuotaColumn = map[string]string{
	"storage_bytes": "storage_bytes_used",
	"cloud_egress":  "cloud_egress_used",
}

// usageDailyColumn maps a metering dimension to its usage_daily column.
var usageDailyColumn = map[string]string{
	"storage_bytes": "storage_bytes",
	"cloud_egress":  "cloud_egress",
}

// AddUsage applies a metering delta to the tenant's cumulative quota usage
// (spec §2/§29). dim is validated against a fixed column whitelist (no injection).
func (s *Store) AddUsage(ctx context.Context, tenantID, dim string, delta int64) error {
	col, ok := usageQuotaColumn[dim]
	if !ok {
		return fmt.Errorf("unknown usage dimension %q", dim)
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE quotas SET `+col+` = GREATEST(0, `+col+` + $2) WHERE tenant_id=$1`, tenantID, delta)
	return err
}

// AddUsageDaily accumulates a metering delta into today's rollup row (spec §29).
func (s *Store) AddUsageDaily(ctx context.Context, tenantID, dim string, delta int64) error {
	col, ok := usageDailyColumn[dim]
	if !ok {
		return nil
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO usage_daily(tenant_id, day, `+col+`) VALUES($1, CURRENT_DATE, $2)
		 ON CONFLICT (tenant_id, day) DO UPDATE SET `+col+` = usage_daily.`+col+` + $2`,
		tenantID, delta)
	return err
}

// UsageDailyRow is one day's usage rollup.
type UsageDailyRow struct {
	Day          time.Time `json:"day"`
	StorageBytes int64     `json:"storageBytes"`
	CloudEgress  int64     `json:"cloudEgress"`
}

// ListUsageDaily returns the last `days` of usage rollups, oldest first (spec §29).
func (s *Store) ListUsageDaily(ctx context.Context, tenantID string, days int) ([]UsageDailyRow, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT day, storage_bytes, cloud_egress
		 FROM usage_daily WHERE tenant_id=$1 AND day >= CURRENT_DATE - $2::int
		 ORDER BY day ASC`, tenantID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageDailyRow
	for rows.Next() {
		var u UsageDailyRow
		if err := rows.Scan(&u.Day, &u.StorageBytes, &u.CloudEgress); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
