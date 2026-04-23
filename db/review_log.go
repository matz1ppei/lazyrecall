package db

import (
	"database/sql"
	"time"
)

// StartReviewSession creates a new review_sessions record and returns its ID.
func StartReviewSession(database *sql.DB, mode string, daySessionNo int) (int64, error) {
	res, err := database.Exec(
		`INSERT INTO review_sessions (started_at, mode, day_session_no) VALUES (?, ?, ?)`,
		time.Now().UTC().Format("2006-01-02 15:04:05"), mode, daySessionNo,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndReviewSession sets ended_at on the given session.
func EndReviewSession(database *sql.DB, sessionID int64) error {
	_, err := database.Exec(
		`UPDATE review_sessions SET ended_at = ? WHERE id = ?`,
		time.Now().UTC().Format("2006-01-02 15:04:05"), sessionID,
	)
	return err
}

// InsertReviewEvent records a single card review within a session.
func InsertReviewEvent(database *sql.DB, sessionID, cardID int64, position int, responseTimeMs int64, correct bool) error {
	correctInt := 0
	if correct {
		correctInt = 1
	}
	_, err := database.Exec(
		`INSERT INTO review_events (review_session_id, card_id, position, response_time_ms, correct) VALUES (?, ?, ?, ?, ?)`,
		sessionID, cardID, position, responseTimeMs, correctInt,
	)
	return err
}

// CountTodayReviewSessions returns the number of review_sessions that have ended today.
// Used to compute day_session_no for the next session.
func CountTodayReviewSessions(database *sql.DB) (int, error) {
	var count int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM review_sessions WHERE date(started_at, 'localtime') = date('now', 'localtime')`,
	).Scan(&count)
	return count, err
}
