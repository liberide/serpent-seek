package httpd

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/liberide/serpent-seek/internal/store"
)

// handleStatsSummary returns dashboard aggregates plus provider status.
func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var (
		summary *store.StatsSummary
		err     error
	)
	if s.isAdmin(r) {
		summary, err = s.store.Summary(ctx, 7)
	} else {
		summary, err = s.store.SummaryForUser(ctx, 7, s.identityUserID(r))
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	providers, _ := s.store.ListProviders(ctx)
	views := make([]providerView, 0, len(providers))
	for _, p := range providers {
		views = append(views, toProviderView(p))
	}
	if summary.Recent == nil {
		summary.Recent = []*store.Request{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":   summary,
		"providers": views,
		"jobs":      s.jobs.Status(),
	})
}

// analyticsMaxDays caps a single analytics request so the daily series cannot
// grow without bound.
const analyticsMaxDays = 366

// handleStatsAnalytics returns aggregated request statistics for a date range,
// with per-provider outcomes and a per-hour distribution.
func (s *Server) handleStatsAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	now := time.Now().UTC()
	from := strings.TrimSpace(q.Get("from"))
	to := strings.TrimSpace(q.Get("to"))
	if from == "" {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}

	fromDate, errFrom := time.Parse("2006-01-02", from)
	toDate, errTo := time.Parse("2006-01-02", to)
	if errFrom != nil || errTo != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_range", "from and to must be YYYY-MM-DD dates")
		return
	}
	if fromDate.After(toDate) {
		fromDate, toDate = toDate, fromDate
	}
	if toDate.Sub(fromDate) > analyticsMaxDays*24*time.Hour {
		fromDate = toDate.AddDate(0, 0, -analyticsMaxDays)
	}
	from = fromDate.Format("2006-01-02")
	to = toDate.Format("2006-01-02")

	filter := store.AnalyticsFilter{
		From:     from,
		To:       to,
		Provider: strings.TrimSpace(q.Get("provider")),
		Status:   strings.TrimSpace(q.Get("status")),
	}

	report, err := s.store.Analytics(ctx, filter)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	configured, _ := s.store.ListProviders(ctx)
	views := mergeAnalyticsProviders(report.Providers, configured)

	writeJSON(w, http.StatusOK, map[string]any{
		"report":    report,
		"providers": views,
		"range":     map[string]string{"from": from, "to": to},
	})
}

// mergeAnalyticsProviders enriches store-level provider stats with provider
// configuration (code/enabled) and appends configured providers that had no
// activity in the period, so the comparison table always lists every provider.
func mergeAnalyticsProviders(stats []store.ProviderStat, providers []*store.Provider) []store.AnalyticsProvider {
	byName := make(map[string]*store.Provider, len(providers))
	for _, p := range providers {
		byName[p.Name] = p
	}
	seen := make(map[string]bool, len(stats))
	out := make([]store.AnalyticsProvider, 0, len(stats)+len(providers))
	for _, stat := range stats {
		if stat.Provider == "" {
			continue
		}
		v := store.AnalyticsProvider{
			Provider: stat.Provider, Total: stat.Total, OK: stat.OK, Empty: stat.Empty,
			Fail: stat.Fail, Skip: stat.Skip, AvgMS: stat.AvgMS, SuccessRate: stat.SuccessRate,
		}
		if p, ok := byName[stat.Provider]; ok {
			v.Code, v.Enabled, v.Configured = p.Code, p.Enabled, true
		}
		seen[stat.Provider] = true
		out = append(out, v)
	}
	for _, p := range providers {
		if seen[p.Name] {
			continue
		}
		out = append(out, store.AnalyticsProvider{
			Provider: p.Name, Code: p.Code, Enabled: p.Enabled, Configured: true,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}
