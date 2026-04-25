package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ippei/lazyrecall/srs"
)

type SessionResult struct {
	CardID int64
	Rating int
}

// ApplySessionResults updates all review rows for a Daily Session in one transaction.
func ApplySessionResults(database *sql.DB, results []SessionResult, sessionID int64) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	for _, result := range results {
		review, err := getOrCreateReviewTx(tx, result.CardID)
		if err != nil {
			return fmt.Errorf("load review for card %d: %w", result.CardID, err)
		}

		rating := srs.RatingAgain
		if result.Rating == 4 {
			rating = srs.RatingGood
		}
		scheduled := srs.Schedule(ReviewToSRS(review), rating, now)
		ApplySRSResult(&review, scheduled)

		lastRating := result.Rating
		review.LastRating = &lastRating
		if err := updateReviewTx(tx, review, nowStr); err != nil {
			return fmt.Errorf("update review for card %d: %w", result.CardID, err)
		}
	}

	if sessionID != 0 {
		if _, err := tx.Exec(
			`UPDATE review_sessions SET ended_at = ? WHERE id = ?`,
			now.UTC().Format("2006-01-02 15:04:05"), sessionID,
		); err != nil {
			return fmt.Errorf("end review session %d: %w", sessionID, err)
		}
	}

	return tx.Commit()
}

func getOrCreateReviewTx(tx *sql.Tx, cardID int64) (Review, error) {
	var r Review
	var lastRating sql.NullInt64
	var reviewedAt sql.NullString
	var lastReview sql.NullString

	err := tx.QueryRow(
		`SELECT id, card_id, due_date, interval, ease_factor, repetitions, last_rating, reviewed_at,
		        stability, difficulty, fsrs_state, lapses, last_review
		 FROM reviews WHERE card_id = ?`,
		cardID,
	).Scan(
		&r.ID, &r.CardID, &r.DueDate, &r.Interval, &r.EaseFactor, &r.Repetitions,
		&lastRating, &reviewedAt,
		&r.Stability, &r.Difficulty, &r.FSRSState, &r.Lapses, &lastReview,
	)

	if err == sql.ErrNoRows {
		res, err := tx.Exec(`INSERT INTO reviews (card_id) VALUES (?)`, cardID)
		if err != nil {
			return Review{}, err
		}
		id, _ := res.LastInsertId()
		return Review{
			ID:         id,
			CardID:     cardID,
			DueDate:    time.Now().Format("2006-01-02 15:04:05"),
			Interval:   1,
			EaseFactor: 2.5,
		}, nil
	}
	if err != nil {
		return Review{}, err
	}

	scanReview(&r, &lastRating, &reviewedAt, &lastReview)
	return r, nil
}

func updateReviewTx(tx *sql.Tx, r Review, now string) error {
	var lastReviewStr *string
	if r.LastReview != nil {
		s := r.LastReview.Format("2006-01-02 15:04:05")
		lastReviewStr = &s
	}
	_, err := tx.Exec(
		`UPDATE reviews
		 SET due_date=?, interval=?, ease_factor=?, repetitions=?, last_rating=?, reviewed_at=?,
		     stability=?, difficulty=?, fsrs_state=?, lapses=?, last_review=?
		 WHERE id=?`,
		r.DueDate, r.Interval, r.EaseFactor, r.Repetitions, r.LastRating, now,
		r.Stability, r.Difficulty, r.FSRSState, r.Lapses, lastReviewStr,
		r.ID,
	)
	return err
}
