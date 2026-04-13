package db

import (
	"database/sql"
	"time"
)

func RecordBatchStat(database *sql.DB, topic string, batchNum, totalBatches, rankStart, rankEnd int, elapsed time.Duration, err error) {
	success := 1
	errMsg := ""
	if err != nil {
		success = 0
		errMsg = err.Error()
	}
	database.Exec(
		`INSERT INTO batch_stats (topic, batch_num, total_batches, rank_start, rank_end, elapsed_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		topic, batchNum, totalBatches, rankStart, rankEnd, elapsed.Milliseconds(), success, errMsg,
	)
}
