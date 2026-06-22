package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/apage/apage/internal/httpx"
)

// handleHealthz is the liveness probe (spec §31): process alive => 200.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is the readiness probe (spec §31): dependencies reachable => 200.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	deps := map[string]string{"db": "ok", "redis": "ok"}
	code := http.StatusOK
	if err := s.db.Ping(r.Context()); err != nil {
		deps["db"] = "down"
		code = http.StatusServiceUnavailable
	}
	if err := s.rdb.Ping(r.Context()); err != nil {
		deps["redis"] = "down"
		code = http.StatusServiceUnavailable
	}
	httpx.JSON(w, code, map[string]any{"ready": code == http.StatusOK, "deps": deps})
}

// checkDomainTXT performs a best-effort TXT ownership verification (spec §28).
func (s *Server) checkDomainTXT(domain, expected string) bool {
	records, err := net.LookupTXT("_apage." + domain)
	if err != nil {
		return false
	}
	for _, rec := range records {
		if strings.TrimSpace(rec) == expected {
			return true
		}
	}
	return false
}

// checkDomainCNAME verifies the domain's CNAME points at our edge target,
// returning the observed canonical name for diagnostics (spec §28 / UI §7.5).
func (s *Server) checkDomainCNAME(domain, expected string) (observed string, ok bool) {
	cname, err := net.LookupCNAME(domain)
	if err != nil {
		return "", false
	}
	observed = normalizeHost(cname)
	return observed, observed == normalizeHost(expected)
}

// normalizeHost lowercases a hostname and strips the trailing root dot so a
// resolver's canonical form compares equal to our expected target.
func normalizeHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}
