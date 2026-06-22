package store

import "context"

// CountOnlineInstances returns the number of agent instances currently online
// across all tenants (observability, spec §18).
func (s *Store) CountOnlineInstances(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM agent_instances WHERE status='online'`).Scan(&n)
	return n, err
}

// CountActiveLinks returns the number of preview links that are currently
// servable (not revoked/frozen/expired) across all tenants (spec §18).
func (s *Store) CountActiveLinks(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM preview_links
		 WHERE revoked_at IS NULL AND frozen_at IS NULL AND (expires_at IS NULL OR expires_at>now())`).Scan(&n)
	return n, err
}
