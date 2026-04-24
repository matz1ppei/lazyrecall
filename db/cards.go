package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Card struct {
	ID                 int64
	Front              string
	Back               string
	Hint               string
	Example            string
	ExampleTranslation string
	ExampleWord        string
	CreatedAt          time.Time
}

// CountCards returns the total number of cards in the database.
// Used at startup to detect the first-run (zero-card) state for onboarding.
func CountCards(database *sql.DB) (int, error) {
	var count int
	err := database.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&count)
	return count, err
}

func CreateCard(db *sql.DB, front, back, hint, example, exampleTranslation, exampleWord string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO cards (front, back, hint, example, example_translation, example_word) VALUES (?, ?, ?, ?, ?, ?)`,
		front, back, hint, example, exampleTranslation, exampleWord,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateCardWithReview creates a card and its initial review row atomically.
func CreateCardWithReview(db *sql.DB, front, back, hint, example, exampleTranslation, exampleWord string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO cards (front, back, hint, example, example_translation, example_word) VALUES (?, ?, ?, ?, ?, ?)`,
		front, back, hint, example, exampleTranslation, exampleWord,
	)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(`INSERT INTO reviews (card_id) VALUES (?)`, id); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func GetCard(db *sql.DB, id int64) (Card, error) {
	row := db.QueryRow(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at FROM cards WHERE id = ?`, id,
	)
	var c Card
	var createdAt string
	if err := row.Scan(&c.ID, &c.Front, &c.Back, &c.Hint, &c.Example, &c.ExampleTranslation, &c.ExampleWord, &createdAt); err != nil {
		return Card{}, err
	}
	c.CreatedAt, _ = parseDBTime(createdAt)
	return c, nil
}

func ListCards(db *sql.DB) ([]Card, error) {
	rows, err := db.Query(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at FROM cards ORDER BY id`,
	)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func ListCardsWithReviewByIDs(db *sql.DB, ids []int64) ([]CardWithReview, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := db.Query(
		fmt.Sprintf(
			`SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at,
			        COALESCE(r.id,0), COALESCE(r.card_id,0), COALESCE(r.due_date,''), COALESCE(r.interval,1),
			        COALESCE(r.ease_factor,2.5), COALESCE(r.repetitions,0), r.last_rating, r.reviewed_at,
			        COALESCE(r.stability,0), COALESCE(r.difficulty,0), COALESCE(r.fsrs_state,0),
			        COALESCE(r.lapses,0), r.last_review
			 FROM cards c
			 LEFT JOIN reviews r ON r.card_id = c.id
			 WHERE c.id IN (%s)`,
			strings.Join(placeholders, ","),
		),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[int64]CardWithReview, len(ids))
	for rows.Next() {
		var cwr CardWithReview
		var createdAt string
		var lastRating sql.NullInt64
		var reviewedAt sql.NullString
		var lastReview sql.NullString
		if err := rows.Scan(
			&cwr.Card.ID, &cwr.Front, &cwr.Back, &cwr.Hint, &cwr.Example, &cwr.Card.ExampleTranslation, &cwr.Card.ExampleWord, &createdAt,
			&cwr.Review.ID, &cwr.Review.CardID, &cwr.Review.DueDate,
			&cwr.Review.Interval, &cwr.Review.EaseFactor, &cwr.Review.Repetitions,
			&lastRating, &reviewedAt,
			&cwr.Review.Stability, &cwr.Review.Difficulty, &cwr.Review.FSRSState,
			&cwr.Review.Lapses, &lastReview,
		); err != nil {
			return nil, err
		}
		cwr.Card.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		scanReview(&cwr.Review, &lastRating, &reviewedAt, &lastReview)
		byID[cwr.Card.ID] = cwr
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]CardWithReview, 0, len(ids))
	for _, id := range ids {
		cwr, ok := byID[id]
		if !ok {
			return nil, sql.ErrNoRows
		}
		result = append(result, cwr)
	}
	return result, nil
}

func FindCardsByFront(db *sql.DB, front string) ([]Card, error) {
	rows, err := db.Query(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at FROM cards WHERE LOWER(front) = LOWER(?)`,
		front,
	)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// DeduplicateCards deletes duplicate cards keeping the one with the lowest ID.
// Returns the number of deleted cards.
func DeduplicateCards(db *sql.DB) (int, error) {
	res, err := db.Exec(`
		DELETE FROM cards
		WHERE id NOT IN (
			SELECT MIN(id) FROM cards GROUP BY LOWER(front)
		)
	`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func UpdateCard(db *sql.DB, id int64, front, back, hint, example, exampleTranslation, exampleWord string) error {
	_, err := db.Exec(
		`UPDATE cards SET front=?, back=?, hint=?, example=?, example_translation=?, example_word=? WHERE id=?`,
		front, back, hint, example, exampleTranslation, exampleWord, id,
	)
	return err
}

func DeleteCard(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM cards WHERE id = ?`, id)
	return err
}

// GetRecentFronts returns the fronts of the most recently created cards (newest first).
func GetRecentFronts(database *sql.DB, limit int) ([]string, error) {
	rows, err := database.Query(
		`SELECT front FROM cards ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fronts []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		fronts = append(fronts, f)
	}
	return fronts, rows.Err()
}

// ListRandomCards returns n cards selected at random from the full card pool.
// If fewer than n cards exist, all are returned.
func ListRandomCards(database *sql.DB, n int) ([]Card, error) {
	rows, err := database.Query(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at FROM cards ORDER BY RANDOM() LIMIT ?`, n,
	)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func GetAllFronts(database *sql.DB) (map[string]bool, error) {
	rows, err := database.Query(`SELECT LOWER(front) FROM cards`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		set[f] = true
	}
	return set, rows.Err()
}

// ListRandomCardsExcluding returns n random cards excluding the given IDs.
func ListRandomCardsExcluding(database *sql.DB, n int, excludeIDs []int64) ([]Card, error) {
	if n <= 0 {
		return nil, nil
	}
	if len(excludeIDs) == 0 {
		return ListRandomCards(database, n)
	}
	placeholders := make([]string, len(excludeIDs))
	args := make([]interface{}, len(excludeIDs)+1)
	for i, id := range excludeIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	args[len(excludeIDs)] = n
	query := `SELECT id, front, back, hint, example, example_translation, example_word, created_at FROM cards WHERE id NOT IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY RANDOM() LIMIT ?`
	rows, err := database.Query(query, args...)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// SelectSessionCards returns up to limit cards for a daily session.
// Due cards are prioritized (sorted by due date); remaining slots are filled with random cards.
func SelectSessionCards(database *sql.DB, limit int) ([]CardWithReview, error) {
	due, err := ListDueCards(database, limit)
	if err != nil {
		return nil, err
	}
	if len(due) >= limit {
		return due, nil
	}
	excludeIDs := make([]int64, len(due))
	for i, c := range due {
		excludeIDs[i] = c.Card.ID
	}
	random, err := ListRandomCardsExcluding(database, limit-len(due), excludeIDs)
	if err != nil {
		return due, nil
	}
	for _, c := range random {
		r, err := GetOrCreateReview(database, c.ID)
		if err != nil {
			continue
		}
		due = append(due, CardWithReview{Card: c, Review: r})
	}
	return due, nil
}

// ListCardsWithTranslation returns cards that have both example and example_translation set, in random order.
func ListCardsWithTranslation(database *sql.DB) ([]Card, error) {
	rows, err := database.Query(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at
		 FROM cards
		 WHERE example != '' AND example_translation != ''
		 ORDER BY RANDOM()`,
	)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// ListCardsNeedingTranslation returns cards with example but no example_translation, ordered by id.
func ListCardsNeedingTranslation(database *sql.DB) ([]Card, error) {
	rows, err := database.Query(
		`SELECT id, front, back, hint, example, example_translation, example_word, created_at
		 FROM cards
		 WHERE example != '' AND example_translation = ''
		 ORDER BY id`,
	)
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
		c.CreatedAt, _ = parseDBTime(createdAt)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// UpdateCardTranslation sets example_translation for the given card id.
func UpdateCardTranslation(database *sql.DB, id int64, translation string) error {
	_, err := database.Exec(`UPDATE cards SET example_translation = ? WHERE id = ?`, translation, id)
	return err
}
