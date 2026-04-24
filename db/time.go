package db

import "time"

// parseDBTime parses a datetime string from SQLite.
// modernc.org/sqlite returns DATETIME columns in RFC3339 format ("2006-01-02T15:04:05Z"),
// but values may also be stored as "2006-01-02 15:04:05". Both are handled here.
func parseDBTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}
