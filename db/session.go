package db

import (
	"database/sql"
	"time"
)

type DailySession struct {
	Date        string
	ReviewDone  bool
	MatchDone   bool
	ReverseDone bool
	BlankDone   bool
}

func GetTodaySession(database *sql.DB) (DailySession, error) {
	today := time.Now().UTC().Format("2006-01-02")
	var s DailySession
	var rd, md, revd, bd int
	err := database.QueryRow(
		`SELECT date, review_done, match_done, reverse_done, blank_done FROM daily_sessions WHERE date = ?`, today,
	).Scan(&s.Date, &rd, &md, &revd, &bd)
	if err == sql.ErrNoRows {
		return DailySession{Date: today}, nil
	}
	if err != nil {
		return DailySession{Date: today}, err
	}
	s.ReviewDone = rd == 1
	s.MatchDone = md == 1
	s.ReverseDone = revd == 1
	s.BlankDone = bd == 1
	return s, nil
}

func MarkReviewDone(database *sql.DB) error {
	_, err := database.Exec(
		`INSERT INTO daily_sessions (date, review_done) VALUES (date('now'), 1)
		 ON CONFLICT(date) DO UPDATE SET review_done = 1`,
	)
	return err
}

func MarkMatchDone(database *sql.DB) error {
	_, err := database.Exec(
		`INSERT INTO daily_sessions (date, match_done) VALUES (date('now'), 1)
		 ON CONFLICT(date) DO UPDATE SET match_done = 1`,
	)
	return err
}

func MarkReverseDone(database *sql.DB) error {
	_, err := database.Exec(
		`INSERT INTO daily_sessions (date, reverse_done) VALUES (date('now'), 1)
		 ON CONFLICT(date) DO UPDATE SET reverse_done = 1`,
	)
	return err
}

func MarkBlankDone(database *sql.DB) error {
	_, err := database.Exec(
		`INSERT INTO daily_sessions (date, blank_done) VALUES (date('now'), 1)
		 ON CONFLICT(date) DO UPDATE SET blank_done = 1`,
	)
	return err
}

// GetRecentSessionDates は過去 days 日間の学習完了日の集合を返す。
// いずれかのフェーズ（review/match/reverse/blank）が完了した日を対象とする。
func GetRecentSessionDates(database *sql.DB, days int) (map[string]bool, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	// date(date) でDATE型をYYYY-MM-DD文字列として取得する（ドライバのtime.Time変換を回避）
	rows, err := database.Query(
		`SELECT date(date) FROM daily_sessions
		 WHERE date >= ?
		   AND (review_done = 1 OR match_done = 1 OR reverse_done = 1 OR blank_done = 1)`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		result[d] = true
	}
	return result, rows.Err()
}
