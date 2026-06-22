package api

import (
	"context"
	"encoding/json"

	"github.com/apage/apage/internal/audit"
)

// audit records an audit entry off the request path (spec §15/§19.7). It pushes
// the entry onto the Redis "audit" queue for the worker to persist — a fast
// LPUSH instead of a synchronous multi-column INSERT — and falls back to a direct
// DB write so an entry is never lost if the queue is unavailable.
func (s *Server) audit(ctx context.Context, e audit.Entry) {
	if s.rdb != nil {
		if b, err := json.Marshal(e); err == nil {
			if err := s.rdb.Enqueue(ctx, "audit", string(b)); err == nil {
				return
			}
		}
	}
	_ = s.db.WriteAudit(ctx, e)
}
