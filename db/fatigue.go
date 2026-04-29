package db

import (
	"database/sql"
	"time"
)

type DailySessionMix struct {
	SelectedCount  int
	OverdueCount   int
	LearningCount  int
	ReviewCount    int
	NewCount       int
	FinalPassCount int
	RetryCardCount int
}

type DailySessionPhaseMetric struct {
	Phase           string
	ItemCount       int
	CorrectCount    int
	DurationSeconds int
	Skipped         bool
}

func ClassifyDailySessionCards(cards []CardWithReview) DailySessionMix {
	mix := DailySessionMix{SelectedCount: len(cards)}
	startOfToday := time.Now().Format("2006-01-02 00:00:00")
	for _, c := range cards {
		switch {
		case c.Review.ReviewedAt == nil:
			mix.NewCount++
		case c.Review.DueDate < startOfToday:
			mix.OverdueCount++
		case c.Review.Stability < 21:
			mix.LearningCount++
		default:
			mix.ReviewCount++
		}
	}
	return mix
}

func SaveDailySessionMix(database *sql.DB, reviewSessionID int64, mix DailySessionMix) error {
	if reviewSessionID == 0 {
		return nil
	}
	_, err := database.Exec(
		`INSERT INTO daily_session_mix
		 (review_session_id, selected_count, overdue_count, learning_count, review_count, new_count, final_pass_count, retry_card_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(review_session_id) DO UPDATE SET
		   selected_count = excluded.selected_count,
		   overdue_count = excluded.overdue_count,
		   learning_count = excluded.learning_count,
		   review_count = excluded.review_count,
		   new_count = excluded.new_count,
		   final_pass_count = excluded.final_pass_count,
		   retry_card_count = excluded.retry_card_count`,
		reviewSessionID, mix.SelectedCount, mix.OverdueCount, mix.LearningCount, mix.ReviewCount, mix.NewCount, mix.FinalPassCount, mix.RetryCardCount,
	)
	return err
}

func SaveDailySessionPhaseMetric(database *sql.DB, reviewSessionID int64, metric DailySessionPhaseMetric) error {
	if reviewSessionID == 0 {
		return nil
	}
	skipped := 0
	if metric.Skipped {
		skipped = 1
	}
	_, err := database.Exec(
		`INSERT INTO daily_session_phase_metrics
		 (review_session_id, phase, item_count, correct_count, duration_seconds, skipped)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(review_session_id, phase) DO UPDATE SET
		   item_count = excluded.item_count,
		   correct_count = excluded.correct_count,
		   duration_seconds = excluded.duration_seconds,
		   skipped = excluded.skipped`,
		reviewSessionID, metric.Phase, metric.ItemCount, metric.CorrectCount, metric.DurationSeconds, skipped,
	)
	return err
}

func saveDailySessionFinalCountsTx(tx *sql.Tx, reviewSessionID int64, finalPassCount, retryCardCount int) error {
	if reviewSessionID == 0 {
		return nil
	}
	_, err := tx.Exec(
		`INSERT INTO daily_session_mix
		 (review_session_id, selected_count, overdue_count, learning_count, review_count, new_count, final_pass_count, retry_card_count)
		 VALUES (?, 0, 0, 0, 0, 0, ?, ?)
		 ON CONFLICT(review_session_id) DO UPDATE SET
		   final_pass_count = excluded.final_pass_count,
		   retry_card_count = excluded.retry_card_count`,
		reviewSessionID, finalPassCount, retryCardCount,
	)
	return err
}
