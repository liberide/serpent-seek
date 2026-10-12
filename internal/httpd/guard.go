package httpd

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
	"github.com/liberide/serpent-seek/internal/notify"
	"github.com/liberide/serpent-seek/internal/store"
)

// searchGuard enforces the optional per-subject search rate limit and daily
// quota on the expensive search endpoints (`/search`, `/mcp`, playground). Both
// limits default to 0 = disabled, so existing deployments are unaffected.
//
// The counter subject is the API key when one is used, otherwise the user or the
// client IP, so sessions and AUTH_ENABLED=false installs are covered too.
func (s *Server) searchGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rateLimit := s.settings.GetInt(ctx, config.KeySearchRateLimit)
		quota := s.settings.GetInt(ctx, config.KeySearchDailyQuota)
		userQuota := s.quotas.get(ctx, s.identityUserID(r))
		if rateLimit <= 0 && quota <= 0 && userQuota == nil {
			next.ServeHTTP(w, r)
			return
		}

		subject := s.usageSubject(r)

		if rateLimit > 0 && !s.searchLimiter.AllowLimit(subject, rateLimit, time.Minute) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-Serpent-Api", "rate_limited")
			writeError(w, r, http.StatusTooManyRequests, "rate_limited", "search rate limit exceeded; retry later")
			return
		}

		// Per-user quota: overrides nothing, adds a per-user ceiling on top
		// of the global limits. Counted against the owning user's subject so
		// every API key and session of that user shares one allowance.
		if userQuota != nil {
			if !s.checkUserQuota(w, r, userQuota) {
				return
			}
		}

		if quota > 0 {
			day := time.Now().UTC().Format("2006-01-02")
			used, err := s.store.GetUsage(ctx, subject, day)
			if err != nil {
				// A counter read failure must not take search down; any real
				// storage problem will surface when the request is persisted.
				s.log.Warn("", "", "usage read failed: "+err.Error())
			} else if used >= quota {
				w.Header().Set("Retry-After", strconv.Itoa(secondsUntilNextUTCDay()))
				w.Header().Set("X-Serpent-Api", "quota_exceeded")
				s.notifyEvent(ctx, "quota_exceeded", map[string]any{
					"target": subject, "subject": subject, "quota": quota,
				})
				writeError(w, r, http.StatusTooManyRequests, "quota_exceeded", "daily search quota exceeded")
				return
			}
			if count, err := s.store.IncrementUsage(ctx, subject, day); err == nil {
				remaining := quota - count
				if remaining < 0 {
					remaining = 0
				}
				w.Header().Set("X-Serpent-Quota-Limit", strconv.Itoa(quota))
				w.Header().Set("X-Serpent-Quota-Remaining", strconv.Itoa(remaining))
			} else {
				s.log.Warn("", "", "usage increment failed: "+err.Error())
			}
		}

		next.ServeHTTP(w, r)
	})
}

// checkUserQuota enforces the per-user quota for the request's owning user.
// It counts usage on the "user:<id>" subject so all of a user's keys and
// sessions share one allowance. On success it records the hit and sets the
// X-Serpent-Quota-* headers; on exhaustion it writes 429 and returns false.
func (s *Server) checkUserQuota(w http.ResponseWriter, r *http.Request, q *store.UserQuota) bool {
	ctx := r.Context()
	userID := s.identityUserID(r)
	subject := "user:" + userID
	now := time.Now()

	used, err := s.quotaUsage(ctx, subject, q, now)
	if err != nil {
		// A counter read failure must not take search down; the global quota
		// and the persisted request still apply.
		s.log.Warn("", "", "user quota read failed: "+err.Error())
	} else if used >= q.Requests {
		retryAfter := int(quotaWindowDuration(q).Seconds())
		resetAt, ok := quotaResetAt(q, now)
		if ok {
			retryAfter = int(time.Until(resetAt).Seconds()) + 1
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("X-Serpent-Quota-Reset-At", resetAt.UTC().Format(time.RFC3339))
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		w.Header().Set("X-Serpent-Api", "quota_exceeded")
		w.Header().Set("X-Serpent-Quota-Limit", strconv.Itoa(q.Requests))
		w.Header().Set("X-Serpent-Quota-Remaining", "0")
		s.notifyEvent(ctx, "quota_exceeded", map[string]any{
			"target": subject, "subject": subject, "quota": q.Requests, "period": q.Period,
		})
		if s.engine != nil && s.engine.Metrics() != nil {
			s.engine.Metrics().RecordQuotaRejected(q.Period)
		}
		writeError(w, r, http.StatusTooManyRequests, "quota_exceeded", "daily search quota exceeded")
		return false
	}

	if resetAt, ok := quotaResetAt(q, now); ok {
		w.Header().Set("X-Serpent-Quota-Reset-At", resetAt.UTC().Format(time.RFC3339))
	}
	if err := s.recordQuotaHit(ctx, subject, q, now); err != nil {
		s.log.Warn("", "", "user quota increment failed: "+err.Error())
	} else {
		remaining := q.Requests - used - 1
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-Serpent-Quota-Limit", strconv.Itoa(q.Requests))
		w.Header().Set("X-Serpent-Quota-Remaining", strconv.Itoa(remaining))
	}
	return true
}

// usageSubject scopes a counter to the API key when one was used, else the user,
// else the client IP.
// notifyEvent posts a webhook for an operational event when WEBHOOK_URL is set.
// It is fire-and-forget so a slow endpoint never delays a search.
func (s *Server) notifyEvent(ctx context.Context, event string, data map[string]any) {
	url := s.settings.GetString(ctx, config.KeyWebhookURL)
	if url == "" {
		return
	}
	go notify.Emit(ctx, url, event, data, s.log)
}

func (s *Server) usageSubject(r *http.Request) string {
	if id := s.identity(r); id != nil {
		if id.KeyID != "" {
			return "key:" + id.KeyID
		}
		if id.User != nil {
			return "user:" + id.User.ID
		}
	}
	return "ip:" + s.clientIP.ClientIP(r)
}

// secondsUntilNextUTCDay returns the seconds until the daily counter resets.
func secondsUntilNextUTCDay() int {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return int(time.Until(next).Seconds()) + 1
}
