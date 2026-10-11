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

// parseAnalyticsRange validates the from/to query dates, applying the shared
// defaults (current month, capped span). On failure it writes the error
// response and reports false.
func (s *Server) parseAnalyticsRange(w http.ResponseWriter, r *http.Request) (from, to string, ok bool) {
	now := time.Now().UTC()
	from = strings.TrimSpace(r.URL.Query().Get("from"))
	to = strings.TrimSpace(r.URL.Query().Get("to"))
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
		return "", "", false
	}
	if fromDate.After(toDate) {
		fromDate, toDate = toDate, fromDate
	}
	if toDate.Sub(fromDate) > analyticsMaxDays*24*time.Hour {
		fromDate = toDate.AddDate(0, 0, -analyticsMaxDays)
	}
	return fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"), true
}

// handleStatsAnalytics returns aggregated request statistics for a date range,
// with per-provider outcomes and a per-hour distribution. The optional user_id
// narrows the report to one owner ("anonymous" for requests without a user).
func (s *Server) handleStatsAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	from, to, ok := s.parseAnalyticsRange(w, r)
	if !ok {
		return
	}

	// Owner filtering for the per-user analytics view. Non-admins may only
	// inspect their own traffic; admins may also select the anonymous bucket.
	userID := strings.TrimSpace(q.Get("user_id"))
	if userID != "" && !s.isAdmin(r) && userID != s.identityUserID(r) {
		writeError(w, r, http.StatusForbidden, "forbidden", "only administrators can view other users' analytics")
		return
	}

	filter := store.AnalyticsFilter{
		From:     from,
		To:       to,
		Provider: strings.TrimSpace(q.Get("provider")),
		Status:   strings.TrimSpace(q.Get("status")),
		UserID:   userID,
	}

	report, err := s.store.Analytics(ctx, filter)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}

	configured, _ := s.store.ListProviders(ctx)
	views := mergeAnalyticsProviders(report.Providers, configured)
	// Cost is "configured" when any provider declares a cost or when historical
	// steps already recorded one; otherwise the UI hides the column so an
	// unconfigured deployment never sees a misleading $0.00.
	report.CostConfigured = report.CostTotal > 0
	for i := range views {
		if views[i].CostConfigured {
			report.CostConfigured = true
			break
		}
	}

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
			Cost: stat.Cost,
		}
		if p, ok := byName[stat.Provider]; ok {
			v.Code, v.Enabled, v.Configured = p.Code, p.Enabled, true
			v.CostConfigured = p.CostPerRequest() > 0 || stat.Cost > 0
		} else {
			v.CostConfigured = stat.Cost > 0
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
			CostConfigured: p.CostPerRequest() > 0,
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

// userStatRow is one leaderboard row: per-owner request aggregates enriched
// with account data (name, role, quota, key count) by the HTTP layer.
type userStatRow struct {
	store.UserStat
	Name      string                 `json:"name"`
	Role      string                 `json:"role,omitempty"`
	Disabled  bool                   `json:"disabled,omitempty"`
	CreatedAt string                 `json:"created_at,omitempty"`
	Exists    bool                   `json:"exists"`
	KeysCount int                    `json:"keys_count"`
	Quota     *store.UserQuotaStatus `json:"quota,omitempty"`
}

// handleStatsUsers returns the per-user analytics leaderboard for a date
// range: request aggregates for every owner (users plus the anonymous bucket),
// merged with account data and the runtime quota status. Administrators only.
func (s *Server) handleStatsUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, ok := s.parseAnalyticsRange(w, r)
	if !ok {
		return
	}
	stats, err := s.store.UsersOverview(ctx, from, to)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	quotas, err := s.store.ListUserQuotas(ctx)
	if err != nil {
		// A quota read failure must not break the leaderboard.
		s.log.Warn("", "", "list quotas failed: "+err.Error())
		quotas = nil
	}
	// Active (non-revoked) key count per owner.
	keyCounts := map[string]int{}
	if keys, err := s.store.ListAPIKeys(ctx, ""); err == nil {
		for _, k := range keys {
			if k.RevokedAt == "" {
				keyCounts[k.UserID]++
			}
		}
	} else {
		s.log.Warn("", "", "list keys failed: "+err.Error())
	}

	rows := make([]userStatRow, 0, len(stats)+len(users))
	byID := make(map[string]int, len(stats))
	for _, stat := range stats {
		row := userStatRow{UserStat: stat, Exists: true}
		if stat.UserID != store.AnonymousUserID {
			if u, err := s.store.GetUser(ctx, stat.UserID); err == nil {
				row.Name, row.Role, row.Disabled, row.CreatedAt = u.Name, u.Role, u.Disabled, u.CreatedAt
			} else {
				// Requests outlive their user (no FK): report the traffic
				// anyway and let the UI mark the account as deleted.
				row.Exists = false
			}
		}
		if q := quotas[row.UserID]; q != nil && q.Requests > 0 {
			row.Quota = s.quotaStatus(ctx, row.UserID, q)
		}
		row.KeysCount = keyCounts[row.UserID]
		byID[row.UserID] = len(rows)
		rows = append(rows, row)
	}
	// Users without traffic in the period still belong on the board (their
	// quota status is what the admin is after). Zero-filled, sorted by name.
	tail := make([]userStatRow, 0, len(users))
	for _, u := range users {
		if _, seen := byID[u.ID]; seen {
			continue
		}
		row := userStatRow{
			UserStat: store.UserStat{UserID: u.ID}, Name: u.Name, Role: u.Role,
			Disabled: u.Disabled, CreatedAt: u.CreatedAt, Exists: true,
			KeysCount: keyCounts[u.ID],
		}
		if q := quotas[u.ID]; q != nil && q.Requests > 0 {
			row.Quota = s.quotaStatus(ctx, u.ID, q)
		}
		tail = append(tail, row)
	}
	sort.SliceStable(tail, func(i, j int) bool { return tail[i].Name < tail[j].Name })
	rows = append(rows, tail...)

	// Cost is "configured" when any provider declares a cost or when historical
	// steps already recorded one (same rule as the analytics report).
	costConfigured := false
	if providers, err := s.store.ListProviders(ctx); err == nil {
		for _, p := range providers {
			if p.CostPerRequest() > 0 {
				costConfigured = true
				break
			}
		}
	}
	totals := struct {
		Requests       int     `json:"requests"`
		OK             int     `json:"ok"`
		Empty          int     `json:"empty"`
		Fail           int     `json:"fail"`
		ActiveUsers    int     `json:"active_users"`
		Cost           float64 `json:"cost"`
		CostConfigured bool    `json:"cost_configured"`
	}{}
	for _, row := range rows {
		totals.Requests += row.Requests
		totals.OK += row.OK
		totals.Empty += row.Empty
		totals.Fail += row.Fail
		totals.Cost += row.Cost
		if row.Requests > 0 && row.UserID != store.AnonymousUserID {
			totals.ActiveUsers++
		}
		if row.Cost > 0 {
			costConfigured = true
		}
	}
	totals.CostConfigured = costConfigured

	writeJSON(w, http.StatusOK, map[string]any{
		"range":  map[string]string{"from": from, "to": to},
		"rows":   rows,
		"totals": totals,
	})
}
