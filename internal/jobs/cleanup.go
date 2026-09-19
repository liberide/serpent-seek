package jobs

import (
	"context"
	"time"

	"github.com/liberide/serpent-seek/internal/config"
)

// CleanupReport summarises a retention cleanup run.
type CleanupReport struct {
	Requests int64 `json:"requests"`
	Logs     int64 `json:"logs"`
	Sessions int64 `json:"sessions"`
	MS       int64 `json:"ms"`
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

	// Refresh today's and yesterday's rollups.
	today := time.Now().UTC()
	_ = m.store.RecomputeDailyStats(ctx, today.Format("2006-01-02"))
	_ = m.store.RecomputeDailyStats(ctx, today.AddDate(0, 0, -1).Format("2006-01-02"))

	report.MS = time.Since(start).Milliseconds()
	m.log.Info("", "", "cleanup removed requests="+itoa(report.Requests)+" logs="+itoa(report.Logs)+
		" sessions="+itoa(report.Sessions)+" took="+itoa(report.MS)+"ms")
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
