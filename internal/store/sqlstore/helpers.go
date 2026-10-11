package sqlstore

import (
	"time"

	"github.com/google/uuid"
)

func newID() string { return uuid.NewString() }

// nullStr returns nil for an empty string so an optional TEXT column stores
// SQL NULL (required for a nullable foreign key reference) rather than an
// empty string.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

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

// dateStartUTC parses a YYYY-MM-DD date into the UTC start-of-day timestamp.
func dateStartUTC(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// dateEndExclusiveUTC parses a YYYY-MM-DD date into the UTC timestamp just after
// that day, so ranges can use a half-open [from, to) comparison.
func dateEndExclusiveUTC(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
}
