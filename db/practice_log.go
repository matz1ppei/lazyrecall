package db

import "database/sql"

type PracticeTodayStats struct {
	Runs    int
	Items   int
	Correct int
}

type PracticeRun struct {
	Mode       string
	FinishedAt string
	Total      int
	Correct    int
}

func LogPracticeRun(database *sql.DB, mode, startedAt, finishedAt string, total, correct int) error {
	_, err := database.Exec(
		`INSERT INTO practice_runs (mode, started_at, finished_at, total, correct) VALUES (?, ?, ?, ?, ?)`,
		mode, startedAt, finishedAt, total, correct,
	)
	return err
}

func GetTodayPracticeStats(database *sql.DB) (PracticeTodayStats, error) {
	var stats PracticeTodayStats
	err := database.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(total), 0), COALESCE(SUM(correct), 0)
		 FROM practice_runs
		 WHERE date(finished_at, 'localtime') = date('now', 'localtime')`,
	).Scan(&stats.Runs, &stats.Items, &stats.Correct)
	return stats, err
}

func ListRecentPracticeRuns(database *sql.DB, limit int) ([]PracticeRun, error) {
	rows, err := database.Query(
		`SELECT mode, finished_at, total, correct
		 FROM practice_runs
		 ORDER BY finished_at DESC, id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []PracticeRun
	for rows.Next() {
		var run PracticeRun
		if err := rows.Scan(&run.Mode, &run.FinishedAt, &run.Total, &run.Correct); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
