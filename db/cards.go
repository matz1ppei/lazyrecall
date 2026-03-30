package db

import (
	"database/sql"
	"time"
)

type Card struct {
	ID        int64
	Front     string
	Back      string
	Hint      string
	CreatedAt time.Time
}

func CreateCard(db *sql.DB, front, back, hint string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO cards (front, back, hint) VALUES (?, ?, ?)`,
		front, back, hint,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetCard(db *sql.DB, id int64) (Card, error) {
	row := db.QueryRow(
		`SELECT id, front, back, hint, created_at FROM cards WHERE id = ?`, id,
	)
	var c Card
	var createdAt string
	if err := row.Scan(&c.ID, &c.Front, &c.Back, &c.Hint, &createdAt); err != nil {
		return Card{}, err
	}
	c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return c, nil
}

func ListCards(db *sql.DB) ([]Card, error) {
	rows, err := db.Query(
		`SELECT id, front, back, hint, created_at FROM cards ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var c Card
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Front, &c.Back, &c.Hint, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func DeleteCard(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM cards WHERE id = ?`, id)
	return err
}
