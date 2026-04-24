package db

import (
	"database/sql"
	"time"
)

type BenchmarkRun struct {
	ID      int64
	RunAt   time.Time
	Total   int
	Correct int
}

// GetBenchmarkCardIDs returns the snapshotted card IDs, or nil if no snapshot exists.
func GetBenchmarkCardIDs(database *sql.DB) ([]int64, error) {
	rows, err := database.Query(`SELECT card_id FROM benchmark_cards ORDER BY card_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetBenchmarkCards replaces the snapshot with the given card IDs.
func SetBenchmarkCards(database *sql.DB, cardIDs []int64) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM benchmark_cards`); err != nil {
		tx.Rollback()
		return err
	}
	for _, id := range cardIDs {
		if _, err := tx.Exec(`INSERT INTO benchmark_cards (card_id) VALUES (?)`, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ListBenchmarkCards returns the snapshotted cards in ID order.
func ListBenchmarkCards(database *sql.DB) ([]Card, error) {
	rows, err := database.Query(`
		SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at
		FROM benchmark_cards bc
		JOIN cards c ON c.id = bc.card_id
		ORDER BY bc.card_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		var c Card
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Front, &c.Back, &c.Hint, &c.Example, &c.ExampleTranslation, &c.ExampleWord, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// InsertBenchmarkRun saves the result of a benchmark run.
func InsertBenchmarkRun(database *sql.DB, runAt time.Time, total, correct int) error {
	_, err := database.Exec(
		`INSERT INTO benchmark_runs (run_at, total, correct) VALUES (?, ?, ?)`,
		runAt.UTC().Format("2006-01-02 15:04:05"), total, correct,
	)
	return err
}

// ListBenchmarkRuns returns all benchmark runs, newest first.
func ListBenchmarkRuns(database *sql.DB) ([]BenchmarkRun, error) {
	rows, err := database.Query(`SELECT id, run_at, total, correct FROM benchmark_runs ORDER BY run_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []BenchmarkRun
	for rows.Next() {
		var r BenchmarkRun
		var runAt string
		if err := rows.Scan(&r.ID, &runAt, &r.Total, &r.Correct); err != nil {
			return nil, err
		}
		r.RunAt, _ = parseDBTime(runAt)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
