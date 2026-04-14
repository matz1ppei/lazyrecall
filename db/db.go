package db

import (
	"database/sql"
	_ "embed"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// Migrations: ignore errors if columns/tables already exist
	db.Exec(`ALTER TABLE cards ADD COLUMN example TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE cards ADD COLUMN example_translation TEXT NOT NULL DEFAULT ''`)
	db.Exec(`CREATE TABLE IF NOT EXISTS daily_sessions (
		date        DATE    PRIMARY KEY,
		review_done INTEGER NOT NULL DEFAULT 0,
		match_done  INTEGER NOT NULL DEFAULT 0,
		blank_done  INTEGER NOT NULL DEFAULT 0
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS batch_stats (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at    DATETIME NOT NULL DEFAULT (datetime('now')),
		topic         TEXT     NOT NULL,
		batch_num     INTEGER  NOT NULL,
		total_batches INTEGER  NOT NULL,
		rank_start    INTEGER  NOT NULL,
		rank_end      INTEGER  NOT NULL,
		elapsed_ms    INTEGER  NOT NULL,
		success       INTEGER  NOT NULL DEFAULT 1,
		error_msg     TEXT     NOT NULL DEFAULT ''
	)`)
	db.Exec(`ALTER TABLE daily_sessions ADD COLUMN reverse_done INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN stability   REAL    NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN difficulty  REAL    NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN fsrs_state  INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN lapses      INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN last_review DATETIME`)
	db.Exec(`ALTER TABLE daily_sessions ADD COLUMN auto_add_done INTEGER NOT NULL DEFAULT 0`)
	return db, nil
}
