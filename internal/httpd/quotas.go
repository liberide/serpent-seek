package httpd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/liberide/serpent-seek/internal/store"
)

// quotaCacheTTL bounds how long a user quota read from the store is reused
// before it is re-read. Quota changes apply within one TTL without any
// explicit invalidation, and admin writes also invalidate immediately.
const quotaCacheTTL = 5 * time.Second

// quotaCache memoises per-user quota rows so the search guard does not hit
// the store on every request. It is safe for concurrent use.
type quotaCache struct {
	store store.Storage

	mu      sync.RWMutex
	entries map[string]quotaCacheEntry
}

type quotaCacheEntry struct {
	quota   *store.UserQuota
	fetched time.Time
}

func newQuotaCache(st store.Storage) *quotaCache {
	return &quotaCache{store: st, entries: map[string]quotaCacheEntry{}}
}

// get returns the user's quota, or nil when the user is unlimited (no row or
// Requests == 0). Results are cached for quotaCacheTTL; store errors are not
// cached so a transient failure does not disable quotas for a whole TTL.
func (c *quotaCache) get(ctx context.Context, userID string) *store.UserQuota {
	if userID == "" {
		return nil
	}
	c.mu.RLock()
	e, ok := c.entries[userID]
	c.mu.RUnlock()
	// A cached entry (quota or "no quota") is valid for the TTL: it lets an
	// unlimited user skip the store lookup on every request.
	if ok && time.Since(e.fetched) < quotaCacheTTL {
		return e.quota
	}
	q, err := c.store.GetUserQuota(ctx, userID)
	if err != nil {
		if err == store.ErrNotFound {
			c.put(userID, nil)
			return nil
		}
		// Transient error: keep serving without a user quota (the global
		// quota still applies). Do not cache.
		return nil
	}
	if q.Requests <= 0 {
		c.put(userID, nil)
		return nil
	}
	c.put(userID, q)
	return q
}

func (c *quotaCache) put(userID string, q *store.UserQuota) {
	c.mu.Lock()
	c.entries[userID] = quotaCacheEntry{quota: q, fetched: time.Now()}
	c.mu.Unlock()
}

// invalidate drops the cached entry so the next request re-reads the store.
func (c *quotaCache) invalidate(userID string) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.mu.Unlock()
}

// quotaPeriodKey returns the usage_daily day key for a calendar quota period
// at the given moment. The key is "<period-start-date>#<period>" so the
// cleanup job's date-prefix comparison (DeleteOldUsage) prunes old periods
// exactly like plain daily counters. The boundary is shifted by tzOffset
// minutes; the result is always a UTC calendar date.
func quotaPeriodKey(q *store.UserQuota, now time.Time) string {
	shifted := now.UTC().Add(time.Duration(q.TzOffset) * time.Minute)
	var start time.Time
	switch q.Period {
	case store.QuotaPeriodWeekly:
		// ISO week starts Monday. Back up to the most recent Monday 00:00.
		weekday := int(shifted.Weekday()) // Sunday=0
		back := (weekday + 6) % 7
		start = time.Date(shifted.Year(), shifted.Month(), shifted.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -back)
	case store.QuotaPeriodMonthly:
		start = time.Date(shifted.Year(), shifted.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // daily and unknown periods reset at midnight
		start = time.Date(shifted.Year(), shifted.Month(), shifted.Day(), 0, 0, 0, 0, time.UTC)
	}
	return fmt.Sprintf("%s#%s", start.Format("2006-01-02"), q.Period)
}

// quotaResetAt returns the UTC moment the current quota period ends (the
// next reset boundary after now), shifted by tzOffset. For rolling quotas it
// returns false: a rolling window has no fixed reset moment.
func quotaResetAt(q *store.UserQuota, now time.Time) (time.Time, bool) {
	shifted := now.UTC().Add(time.Duration(q.TzOffset) * time.Minute)
	midnight := time.Date(shifted.Year(), shifted.Month(), shifted.Day(), 0, 0, 0, 0, time.UTC)
	switch q.Period {
	case store.QuotaPeriodWeekly:
		weekday := int(shifted.Weekday())
		forward := (7 - (weekday+6)%7) % 7
		if forward == 0 {
			forward = 7
		}
		return midnight.AddDate(0, 0, forward).Add(-time.Duration(q.TzOffset) * time.Minute), true
	case store.QuotaPeriodMonthly:
		// First day of next month, not "same day next month".
		firstOfNext := time.Date(shifted.Year(), shifted.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		return firstOfNext.Add(-time.Duration(q.TzOffset) * time.Minute), true
	case store.QuotaPeriodRolling:
		return time.Time{}, false
	default: // daily
		return midnight.AddDate(0, 0, 1).Add(-time.Duration(q.TzOffset) * time.Minute), true
	}
}

// quotaWindowDuration returns the rolling window length, clamped to a sane
// range so a corrupt row cannot produce a zero or negative window.
func quotaWindowDuration(q *store.UserQuota) time.Duration {
	h := q.WindowHours
	if h <= 0 {
		h = 24
	}
	if h > 720 {
		h = 720
	}
	return time.Duration(h) * time.Hour
}

// quotaUsage reads the subject's current usage for the quota: the calendar
// counter for daily/weekly/monthly, or the rolling hit count for rolling.
func (s *Server) quotaUsage(ctx context.Context, subject string, q *store.UserQuota, now time.Time) (int, error) {
	if q.Period == store.QuotaPeriodRolling {
		since := now.UTC().Add(-quotaWindowDuration(q)).Format(time.RFC3339Nano)
		return s.store.CountRollingHits(ctx, subject, since)
	}
	return s.store.GetUsage(ctx, subject, quotaPeriodKey(q, now))
}

// recordQuotaHit records one admitted request against the quota subject.
func (s *Server) recordQuotaHit(ctx context.Context, subject string, q *store.UserQuota, now time.Time) error {
	if q.Period == store.QuotaPeriodRolling {
		return s.store.AddRollingHit(ctx, subject, now.UTC().Format(time.RFC3339Nano))
	}
	_, err := s.store.IncrementUsage(ctx, subject, quotaPeriodKey(q, now))
	return err
}
