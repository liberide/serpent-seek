package sqlstore

import (
	"time"

	"github.com/google/uuid"
)

func newID() string { return uuid.NewString() }

func dayStartUTC() string {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
}

func dayStartUTCMinus(days int) string {
	now := time.Now().UTC().AddDate(0, 0, -days)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
}

func dateOnlyMinus(days int) string {
	now := time.Now().UTC().AddDate(0, 0, -days)
	return now.Format("2006-01-02")
}
