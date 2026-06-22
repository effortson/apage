package api

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// MetricsHandler serves the API's internal observability endpoints on a separate
// listener (MetricsAddr), so business metrics are not exposed on the public host
// (spec §18/§31). Mount this on cfg.MetricsAddr.
func (s *Server) MetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetricsExposition)
	return mux
}

// handleMetricsExposition writes Prometheus-text business metrics (spec §18).
func (s *Server) handleMetricsExposition(w http.ResponseWriter, r *http.Request) {
	online, _ := s.db.CountOnlineInstances(r.Context())
	active, _ := s.db.CountActiveLinks(r.Context())
	scanQ, _ := s.rdb.QueueLen(r.Context(), "scan")
	deleteQ, _ := s.rdb.QueueLen(r.Context(), "delete")
	auditQ, _ := s.rdb.QueueLen(r.Context(), "audit")

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	g := func(name, help string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, v)
	}
	g("apage_agent_online_count", "Agent instances currently online", int64(online))
	g("apage_active_links_count", "Preview links currently servable", int64(active))
	g("apage_scan_queue_depth", "Pending file-scan jobs", scanQ)
	g("apage_delete_queue_depth", "Pending object-delete jobs", deleteQ)
	g("apage_audit_queue_depth", "Pending audit-write jobs", auditQ)
	fmt.Fprintf(w, "# HELP apage_preview_access_total Preview link accesses served\n")
	fmt.Fprintf(w, "# TYPE apage_preview_access_total counter\n")
	fmt.Fprintf(w, "apage_preview_access_total %d\n", atomic.LoadInt64(&s.previewAccessTotal))
}
