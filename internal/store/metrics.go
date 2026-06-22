package store

import "context"

// CountInstances returns the total number of agent instances across all tenants
// (observability, spec §18). Cloud-only instances have no live-connection state.
func (s *Store) CountInstances(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM agent_instances`).Scan(&n)
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
