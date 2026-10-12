package jobs

import (
	"context"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
)

// CleanupReport summarises a retention cleanup run.
type CleanupReport struct {
	Requests    int64 `json:"requests"`
	Logs        int64 `json:"logs"`
	Sessions    int64 `json:"sessions"`
	Usage       int64 `json:"usage"`
	RollingHits int64 `json:"rolling_hits"`
	Audit       int64 `json:"audit"`
	MS          int64 `json:"ms"`
}

// Cleanup deletes history and logs past their retention windows, plus expired
// sessions. A retention value of 0 disables that deletion.
func (m *Manager) Cleanup(ctx context.Context) (*CleanupReport, error) {
	start := time.Now()
	report := &CleanupReport{}

	historyDays := m.settings.GetInt(ctx, config.KeyHistoryRetention)
	if historyDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -historyDays).Format(time.RFC3339Nano)
		n, err := m.store.DeleteOldRequests(ctx, cutoff)
		if err != nil {
			return nil, err
		}
		report.Requests = n
	}

	logDays := m.settings.GetInt(ctx, config.KeyLogsRetention)
	if logDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -logDays).Format(time.RFC3339Nano)
		n, err := m.store.DeleteOldLogs(ctx, cutoff)
		if err != nil {
			return nil, err
		}
		report.Logs = n
	}

	n, err := m.store.DeleteExpiredSessions(ctx, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	report.Sessions = n

	// Search quota counters only matter for the current period; keep a
	// bounded, configurable window so the table cannot grow forever (0
	// disables pruning). The date-prefix comparison also prunes per-user
	// quota period keys ("<start-date>#<period>").
	if usageDays := m.settings.GetInt(ctx, config.KeyUsageRetention); usageDays > 0 {
		usageCutoff := time.Now().UTC().AddDate(0, 0, -usageDays).Format("2006-01-02")
		n, err = m.store.DeleteOldUsage(ctx, usageCutoff)
		if err != nil {
			return nil, err
		}
		report.Usage = n
	}

	// Rolling-window quota hits are self-limiting (one row per admitted
	// request), but prune them alongside the daily counters. The longest
	// allowed window is 720h, so the usage retention window always covers it.
	if usageDays := m.settings.GetInt(ctx, config.KeyUsageRetention); usageDays > 0 {
		rollingCutoff := time.Now().UTC().AddDate(0, 0, -usageDays).Format(time.RFC3339Nano)
		n, err = m.store.DeleteOldRollingHits(ctx, rollingCutoff)
		if err != nil {
			return nil, err
		}
		report.RollingHits = n
	}

	// Audit entries outlive logs; keep a longer configurable window.
	if auditDays := m.settings.GetInt(ctx, config.KeyAuditRetention); auditDays > 0 {
		auditCutoff := time.Now().UTC().AddDate(0, 0, -auditDays).Format(time.RFC3339Nano)
		n, err = m.store.DeleteOldAudit(ctx, auditCutoff)
		if err != nil {
			return nil, err
		}
		report.Audit = n
	}

	// Refresh today's and yesterday's rollups.
	today := time.Now().UTC()
	_ = m.store.RecomputeDailyStats(ctx, today.Format("2006-01-02"))
	_ = m.store.RecomputeDailyStats(ctx, today.AddDate(0, 0, -1).Format("2006-01-02"))

	report.MS = time.Since(start).Milliseconds()
	m.log.Info("", "", "cleanup removed requests="+itoa(report.Requests)+" logs="+itoa(report.Logs)+
		" sessions="+itoa(report.Sessions)+" usage="+itoa(report.Usage)+" audit="+itoa(report.Audit)+" took="+itoa(report.MS)+"ms")
	return report, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
