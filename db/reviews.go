package db

import (
	"database/sql"
	"time"

	"github.com/ippei/lazyrecall/srs"
)

type Review struct {
	ID          int64
	CardID      int64
	DueDate     string // "YYYY-MM-DD" — FSRS Due rounded to day
	Interval    int    // FSRS ScheduledDays; kept for backward compat
	EaseFactor  float64
	Repetitions int
	LastRating  *int
	ReviewedAt  *time.Time

	// FSRS fields
	Stability  float64
	Difficulty float64
	FSRSState  int
	Lapses     int
	LastReview *time.Time
}

type CardWithReview struct {
	Card
	Review
}

func GetOrCreateReview(db *sql.DB, cardID int64) (Review, error) {
	var r Review
	var lastRating sql.NullInt64
	var reviewedAt sql.NullString
	var lastReview sql.NullString

	err := db.QueryRow(
		`SELECT id, card_id, due_date, interval, ease_factor, repetitions, last_rating, reviewed_at,
		        stability, difficulty, fsrs_state, lapses, last_review
		 FROM reviews WHERE card_id = ?`, cardID,
	).Scan(
		&r.ID, &r.CardID, &r.DueDate, &r.Interval, &r.EaseFactor, &r.Repetitions,
		&lastRating, &reviewedAt,
		&r.Stability, &r.Difficulty, &r.FSRSState, &r.Lapses, &lastReview,
	)

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
	if lastReview.Valid {
		t := parseDatetime(lastReview.String)
		r.LastReview = &t
	}
	return r, nil
}

// parseDatetime handles both "2006-01-02 15:04:05" and "2006-01-02T15:04:05Z" SQLite formats.
func parseDatetime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func UpdateReview(db *sql.DB, r Review) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	var lastReviewStr *string
	if r.LastReview != nil {
		s := r.LastReview.Format("2006-01-02 15:04:05")
		lastReviewStr = &s
	}
	_, err := db.Exec(
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

func scanReview(
	destReview *Review,
	lastRating *sql.NullInt64,
	reviewedAt *sql.NullString,
	lastReview *sql.NullString,
) {
	if lastRating.Valid {
		v := int(lastRating.Int64)
		destReview.LastRating = &v
	}
	if reviewedAt.Valid {
		t := parseDatetime(reviewedAt.String)
		destReview.ReviewedAt = &t
	}
	if lastReview.Valid {
		t := parseDatetime(lastReview.String)
		destReview.LastReview = &t
	}
}

func ListAllCardsWithReview(db *sql.DB) ([]CardWithReview, error) {
	rows, err := db.Query(
		`SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at,
		        COALESCE(r.id,0), COALESCE(r.card_id,0), COALESCE(r.due_date,''), COALESCE(r.interval,1),
		        COALESCE(r.ease_factor,2.5), COALESCE(r.repetitions,0), r.last_rating, r.reviewed_at,
		        COALESCE(r.stability,0), COALESCE(r.difficulty,0), COALESCE(r.fsrs_state,0),
		        COALESCE(r.lapses,0), r.last_review
		 FROM cards c
		 LEFT JOIN reviews r ON r.card_id = c.id
		 ORDER BY c.id`,
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
		result = append(result, cwr)
	}
	return result, rows.Err()
}

func CountDueCards(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE due_date <= date('now', 'localtime')`).Scan(&n)
	return n, err
}

func SetDueToday(db *sql.DB, cardIDs []int64) error {
	today := time.Now().Format("2006-01-02")
	for _, id := range cardIDs {
		if _, err := db.Exec(`UPDATE reviews SET due_date = ? WHERE card_id = ?`, today, id); err != nil {
			return err
		}
	}
	return nil
}

func CountOverdueCards(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE due_date < date('now', 'localtime')`).Scan(&n)
	return n, err
}

type ReviewStats struct {
	TotalCards    int
	NewCards      int // reviewed_at IS NULL
	LearningCards int // stability < 21 and reviewed_at IS NOT NULL
	MatureCards   int // stability >= 21
	ReviewedToday int
	CorrectToday  int
	Streak        int
}

func GetReviewStats(db *sql.DB) (ReviewStats, error) {
	var s ReviewStats

	// card maturity breakdown using stability (FSRS) instead of interval (SM-2)
	err := db.QueryRow(`
		SELECT
			COUNT(*),
			SUM(CASE WHEN r.reviewed_at IS NULL THEN 1 ELSE 0 END),
			SUM(CASE WHEN r.reviewed_at IS NOT NULL AND r.stability < 21 THEN 1 ELSE 0 END),
			SUM(CASE WHEN r.stability >= 21 THEN 1 ELSE 0 END)
		FROM cards c
		LEFT JOIN reviews r ON r.card_id = c.id
	`).Scan(&s.TotalCards, &s.NewCards, &s.LearningCards, &s.MatureCards)
	if err != nil {
		return s, err
	}

	// today's reviewed and correct count
	err = db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN last_rating = 4 THEN 1 ELSE 0 END), 0)
		FROM reviews
		WHERE date(reviewed_at) = date('now', 'localtime')
	`).Scan(&s.ReviewedToday, &s.CorrectToday)
	if err != nil {
		return s, err
	}

	// streak: based on daily_sessions (any phase counts)
	// date(date) でDATE型をYYYY-MM-DD文字列として取得する（ドライバのtime.Time変換を回避）
	rows, err := db.Query(`
		SELECT date(date) FROM daily_sessions
		WHERE review_done = 1 OR match_done = 1 OR blank_done = 1
		ORDER BY date DESC
		LIMIT 365
	`)
	if err != nil {
		return s, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return s, err
		}
		dates = append(dates, d)
	}
	s.Streak = calcStreak(dates)

	return s, rows.Err()
}

func calcStreak(dates []string) int {
	if len(dates) == 0 {
		return 0
	}
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// streak must include today or yesterday to be active
	if dates[0] != today && dates[0] != yesterday {
		return 0
	}

	streak := 1
	for i := 1; i < len(dates); i++ {
		prev, _ := time.Parse("2006-01-02", dates[i-1])
		curr, _ := time.Parse("2006-01-02", dates[i])
		if prev.AddDate(0, 0, -1).Format("2006-01-02") == curr.Format("2006-01-02") {
			streak++
		} else {
			break
		}
	}
	return streak
}

func CountReviewedToday(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE date(reviewed_at) = date('now', 'localtime')`).Scan(&n)
	return n, err
}

func ListDueCards(db *sql.DB, limit int) ([]CardWithReview, error) {
	rows, err := db.Query(
		`SELECT c.id, c.front, c.back, c.hint, c.example, c.example_translation, c.example_word, c.created_at,
		        r.id, r.card_id, r.due_date, r.interval, r.ease_factor, r.repetitions, r.last_rating, r.reviewed_at,
		        r.stability, r.difficulty, r.fsrs_state, r.lapses, r.last_review
		 FROM cards c
		 JOIN reviews r ON r.card_id = c.id
		 WHERE r.due_date <= date('now', 'localtime')
		 ORDER BY r.due_date, c.id
		 LIMIT ?`,
		limit,
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
		result = append(result, cwr)
	}
	return result, rows.Err()
}

// ReviewToSRS converts a db.Review to srs.CardState for FSRS scheduling.
func ReviewToSRS(r Review) srs.CardState {
	state := srs.CardState{
		Stability:     r.Stability,
		Difficulty:    r.Difficulty,
		State:         r.FSRSState,
		Reps:          r.Repetitions,
		Lapses:        r.Lapses,
		ScheduledDays: r.Interval,
	}
	if r.LastReview != nil {
		state.LastReview = *r.LastReview
	}
	return state
}

// ApplySRSResult writes a srs.Result back into a Review, including DueDate, Interval, and FSRS fields.
func ApplySRSResult(r *Review, res srs.Result) {
	r.DueDate = res.Due.Format("2006-01-02")
	r.Interval = res.ScheduledDays
	r.Stability = res.Stability
	r.Difficulty = res.Difficulty
	r.FSRSState = res.State
	r.Repetitions = res.Reps
	r.Lapses = res.Lapses
	t := res.LastReview
	r.LastReview = &t
}
