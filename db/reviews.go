package db

import (
	"database/sql"
	"time"
)

type Review struct {
	ID          int64
	CardID      int64
	DueDate     string // "YYYY-MM-DD"
	Interval    int
	EaseFactor  float64
	Repetitions int
	LastRating  *int
	ReviewedAt  *time.Time
}

type CardWithReview struct {
	Card
	Review
}

func GetOrCreateReview(db *sql.DB, cardID int64) (Review, error) {
	var r Review
	var lastRating sql.NullInt64
	var reviewedAt sql.NullString

	err := db.QueryRow(
		`SELECT id, card_id, due_date, interval, ease_factor, repetitions, last_rating, reviewed_at
		 FROM reviews WHERE card_id = ?`, cardID,
	).Scan(&r.ID, &r.CardID, &r.DueDate, &r.Interval, &r.EaseFactor, &r.Repetitions, &lastRating, &reviewedAt)

	if err == sql.ErrNoRows {
		res, err := db.Exec(
			`INSERT INTO reviews (card_id) VALUES (?)`, cardID,
		)
		if err != nil {
			return Review{}, err
		}
		id, _ := res.LastInsertId()
		return Review{
			ID:         id,
			CardID:     cardID,
			DueDate:    time.Now().Format("2006-01-02"),
			Interval:   1,
			EaseFactor: 2.5,
		}, nil
	}
	if err != nil {
		return Review{}, err
	}

	if lastRating.Valid {
		v := int(lastRating.Int64)
		r.LastRating = &v
	}
	if reviewedAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", reviewedAt.String)
		r.ReviewedAt = &t
	}
	return r, nil
}

func UpdateReview(db *sql.DB, r Review) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.Exec(
		`UPDATE reviews SET due_date=?, interval=?, ease_factor=?, repetitions=?, last_rating=?, reviewed_at=?
		 WHERE id=?`,
		r.DueDate, r.Interval, r.EaseFactor, r.Repetitions, r.LastRating, now, r.ID,
	)
	return err
}

func ListDueCards(db *sql.DB) ([]CardWithReview, error) {
	rows, err := db.Query(
		`SELECT c.id, c.front, c.back, c.hint, c.created_at,
		        r.id, r.card_id, r.due_date, r.interval, r.ease_factor, r.repetitions, r.last_rating, r.reviewed_at
		 FROM cards c
		 JOIN reviews r ON r.card_id = c.id
		 WHERE r.due_date <= date('now')
		 ORDER BY r.due_date, c.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CardWithReview
	for rows.Next() {
		var cwr CardWithReview
		var createdAt string
		var lastRating sql.NullInt64
		var reviewedAt sql.NullString
		if err := rows.Scan(
			&cwr.Card.ID, &cwr.Front, &cwr.Back, &cwr.Hint, &createdAt,
			&cwr.Review.ID, &cwr.Review.CardID, &cwr.Review.DueDate,
			&cwr.Review.Interval, &cwr.Review.EaseFactor, &cwr.Review.Repetitions,
			&lastRating, &reviewedAt,
		); err != nil {
			return nil, err
		}
		cwr.Card.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		if lastRating.Valid {
			v := int(lastRating.Int64)
			cwr.Review.LastRating = &v
		}
		if reviewedAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", reviewedAt.String)
			cwr.Review.ReviewedAt = &t
		}
		result = append(result, cwr)
	}
	return result, rows.Err()
}
