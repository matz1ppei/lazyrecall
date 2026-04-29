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

type SuspiciousCard struct {
	CardWithReview
	Reason string
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

func ListSuspiciousCards(db *sql.DB) ([]SuspiciousCard, error) {
	rows, err := db.Query(
		`SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at,
		        COALESCE(r.id,0), COALESCE(r.card_id,0), COALESCE(r.due_date,''), COALESCE(r.interval,1),
		        COALESCE(r.ease_factor,2.5), COALESCE(r.repetitions,0), r.last_rating, r.reviewed_at,
		        COALESCE(r.stability,0), COALESCE(r.difficulty,0), COALESCE(r.fsrs_state,0),
		        COALESCE(r.lapses,0), r.last_review
		 FROM cards c
		 LEFT JOIN reviews r ON r.card_id = c.id
		 WHERE (
		     c.example_word != '' AND LOWER(c.example_word) != LOWER(c.front)
		 ) OR (
		     c.example != '' AND INSTR(LOWER(c.example), LOWER(c.front)) = 0
		 )
		 ORDER BY c.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []SuspiciousCard
	for rows.Next() {
		var c SuspiciousCard
		var createdAt string
		var lastRating sql.NullInt64
		var reviewedAt sql.NullString
		var lastReview sql.NullString
		if err := rows.Scan(
			&c.Card.ID, &c.Front, &c.Back, &c.Hint, &c.Example, &c.Card.ExampleTranslation, &c.Card.ExampleWord, &createdAt,
			&c.Review.ID, &c.Review.CardID, &c.Review.DueDate,
			&c.Review.Interval, &c.Review.EaseFactor, &c.Review.Repetitions,
			&lastRating, &reviewedAt,
			&c.Review.Stability, &c.Review.Difficulty, &c.Review.FSRSState,
			&c.Review.Lapses, &lastReview,
		); err != nil {
			return nil, err
		}
		c.Card.CreatedAt, _ = parseDBTime(createdAt)
		scanReview(&c.Review, &lastRating, &reviewedAt, &lastReview)
		c.Reason = SuspiciousReason(c.Card)
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func SuspiciousReason(card Card) string {
	reasons := make([]string, 0, 2)
	if card.ExampleWord != "" && !strings.EqualFold(card.ExampleWord, card.Front) {
		reasons = append(reasons, "example word mismatch")
	}
	if card.Example != "" && !strings.Contains(strings.ToLower(card.Example), strings.ToLower(card.Front)) {
		reasons = append(reasons, "front missing in example")
	}
	return strings.Join(reasons, "; ")
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

const maxNewCardsPerSession = 2

// SelectSessionCards returns up to limit cards for a daily session.
// Overdue and learning due cards come first; new cards are capped per session.
func SelectSessionCards(database *sql.DB, limit int) ([]CardWithReview, error) {
	due, err := ListDueCards(database, 0)
	if err != nil {
		return nil, err
	}
	selected := make([]CardWithReview, 0, limit)
	excluded := make(map[int64]bool, limit)

	appendCards := func(cards []CardWithReview) {
		for _, c := range cards {
			if len(selected) >= limit || excluded[c.Card.ID] {
				continue
			}
			selected = append(selected, c)
			excluded[c.Card.ID] = true
		}
	}

	var overdue, learningDue, reviewDue []CardWithReview
	startOfToday := time.Now().Format("2006-01-02 00:00:00")
	for _, c := range due {
		switch {
		case c.Review.ReviewedAt == nil:
			// Brand-new cards are handled by the capped "new" slot later.
			continue
		case c.Review.DueDate < startOfToday:
			overdue = append(overdue, c)
		case c.Review.ReviewedAt != nil && c.Review.Stability < 21:
			learningDue = append(learningDue, c)
		default:
			reviewDue = append(reviewDue, c)
		}
	}

	appendCards(overdue)
	appendCards(learningDue)
	appendCards(reviewDue)
	if len(selected) >= limit {
		return selected[:limit], nil
	}

	excludeIDs := make([]int64, 0, len(excluded))
	for id := range excluded {
		excludeIDs = append(excludeIDs, id)
	}

	newLimit := limit - len(selected)
	if newLimit > maxNewCardsPerSession {
		newLimit = maxNewCardsPerSession
	}
	if newLimit > 0 {
		newCards, err := listRandomCardsWithReviewExcluding(database, newLimit, excludeIDs, true)
		if err == nil {
			appendCards(newCards)
			excludeIDs = excludeIDs[:0]
			for id := range excluded {
				excludeIDs = append(excludeIDs, id)
			}
		}
	}
	if len(selected) >= limit {
		return selected[:limit], nil
	}

	random, err := listRandomCardsWithReviewExcluding(database, limit-len(selected), excludeIDs, false)
	if err != nil {
		return selected, nil
	}
	appendCards(random)
	return selected, nil
}

func listRandomCardsWithReviewExcluding(database *sql.DB, n int, excludeIDs []int64, newOnly bool) ([]CardWithReview, error) {
	if n <= 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(excludeIDs))
	args := make([]any, 0, len(excludeIDs)+1)
	for _, id := range excludeIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	query := `SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at,
		         r.id, r.card_id, r.due_date, r.interval, r.ease_factor, r.repetitions, r.last_rating, r.reviewed_at,
		         r.stability, r.difficulty, r.fsrs_state, r.lapses, r.last_review
		  FROM cards c
		  JOIN reviews r ON r.card_id = c.id
		  WHERE 1 = 1`
	if len(placeholders) > 0 {
		query += ` AND c.id NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	if newOnly {
		query += ` AND r.reviewed_at IS NULL`
	} else {
		query += ` AND r.reviewed_at IS NOT NULL`
	}
	query += ` ORDER BY RANDOM() LIMIT ?`
	args = append(args, n)

	rows, err := database.Query(query, args...)
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
		cwr.Card.CreatedAt, _ = parseDBTime(createdAt)
		scanReview(&cwr.Review, &lastRating, &reviewedAt, &lastReview)
		result = append(result, cwr)
	}
	return result, rows.Err()
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
