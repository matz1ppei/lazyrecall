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
	today := time.Now().Format("2006-01-02")
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
