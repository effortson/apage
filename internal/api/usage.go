package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/store"
)

// handleUsage returns the tenant's usage vs limits for the current period (spec §29).
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin") // admin can see usage; owner sees billing (spec §7.7)
	if au == nil {
		return
	}
	q, err := s.db.QuotaFor(r.Context(), au.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"plan":        q.Plan,
		"periodStart": q.PeriodStart,
		"periodEnd":   q.PeriodStart.AddDate(0, 1, 0),
		"metrics": map[string]any{
			"instances":     metric(int64(countUsed(s, r, au.TenantID)), int64(q.InstanceLimit)),
			"storageBytes":  metric(q.StorageBytesUsed, q.StorageBytesLimit),
			"cloudEgress":   metric(q.CloudEgressUsed, q.CloudEgressLimit),
			"customDomains": metric(int64(q.CustomDomainUsed), int64(q.CustomDomainLimit)),
		},
	})
}

func metric(used, limit int64) map[string]int64 {
	return map[string]int64{"used": used, "limit": limit}
}

// handleUsageTimeseries returns the tenant's daily usage trend (spec §29 / UI §7.7).
func (s *Server) handleUsageTimeseries(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			days = n
		}
	}
	rows, err := s.db.ListUsageDaily(r.Context(), au.TenantID, days)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if rows == nil {
		rows = []store.UsageDailyRow{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"days": days, "series": rows})
}

// planPrices are the published monthly prices in USD cents (spec §20).
var planPrices = map[string]int{"lite": 0, "starter": 1900, "pro": 4900, "team": 9900}

var planOrder = []string{"lite", "starter", "pro", "team"}

func planUpgrades(plan string) []string {
	for i, p := range planOrder {
		if p == plan {
			return planOrder[i+1:]
		}
	}
	return nil
}

// handleBilling returns plan, price, current usage, and upgrade options. Billing
// is owner-only (spec §29 / UI §7.7 RBAC).
func (s *Server) handleBilling(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "owner")
	if au == nil {
		return
	}
	q, err := s.db.QuotaFor(r.Context(), au.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"plan":        q.Plan,
		"price":       map[string]any{"monthlyCents": planPrices[q.Plan], "currency": "USD"},
		"periodStart": q.PeriodStart,
		"periodEnd":   q.PeriodStart.AddDate(0, 1, 0),
		"usage": map[string]any{
			"storageBytes": metric(q.StorageBytesUsed, q.StorageBytesLimit),
			"cloudEgress":  metric(q.CloudEgressUsed, q.CloudEgressLimit),
		},
		"upgradeOptions": planUpgrades(q.Plan),
		// Over-quota prompts an upgrade and never silently charges (spec §2/§20).
		"autoCharge": false,
	})
}

func countUsed(s *Server, r *http.Request, tenantID string) int {
	items, err := s.db.ListInstances(r.Context(), tenantID, httpx.Page{Limit: 100, Order: "desc"})
	if err != nil {
		return 0
	}
	return len(items)
}

// handleListAudit returns audit logs (spec §14, admin/owner only).
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	au := requireRole(w, r, "admin")
	if au == nil {
		return
	}
	p, err := httpx.ParsePage(r)
	if err != nil {
		httpx.BadRequest(w, r, err.Error())
		return
	}
	q := r.URL.Query()
	items, err := s.db.ListAudit(r.Context(), au.TenantID, p,
		q.Get("event"), q.Get("resourceType"), q.Get("resourceId"), q.Get("actorType"))
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, p.Limit, func(a store.AuditLog) (time.Time, string) {
		return a.CreatedAt, a.EventID
	}))
}
