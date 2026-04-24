package db

import (
	"database/sql"
	"time"
)

// CountAllReviewSessions returns the total number of review_sessions rows.
func CountAllReviewSessions(database *sql.DB) (int, error) {
	var count int
	err := database.QueryRow(`SELECT COUNT(*) FROM review_sessions`).Scan(&count)
	return count, err
}

// FirstBenchmarkRunAt returns the time of the earliest benchmark_runs record.
// Returns zero time and nil error if no runs exist.
func FirstBenchmarkRunAt(database *sql.DB) (time.Time, error) {
	var runAt string
	err := database.QueryRow(`SELECT run_at FROM benchmark_runs ORDER BY run_at ASC LIMIT 1`).Scan(&runAt)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, _ := parseDBTime(runAt)
	return t, nil
}

// GetMilestoneInt returns the int_value for a key, defaulting to 0 if not set.
func GetMilestoneInt(database *sql.DB, key string) (int, error) {
	var val int
	err := database.QueryRow(`SELECT int_value FROM analysis_milestones WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return val, err
}

// SetMilestoneInt upserts the int_value for a key.
func SetMilestoneInt(database *sql.DB, key string, val int) error {
	_, err := database.Exec(
		`INSERT INTO analysis_milestones (key, int_value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET int_value = excluded.int_value`,
		key, val,
	)
	return err
}
