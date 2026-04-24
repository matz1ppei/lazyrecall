package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type DailySessionSnapshot struct {
	Date              string    `json:"date"`
	CardIDs           []int64   `json:"card_ids"`
	ReviewSessionID   int64     `json:"review_session_id"`
	Phase             string    `json:"phase"`
	ReviewCorrectIDs  []int64   `json:"review_correct_ids"`
	ReverseCorrectIDs []int64   `json:"reverse_correct_ids"`
	MatchWrongIDs     []int64   `json:"match_wrong_ids"`
	BlankCorrectIDs   []int64   `json:"blank_correct_ids"`
	BlankSkipped      bool      `json:"blank_skipped"`
	RetryCardIDs      []int64   `json:"retry_card_ids"`
	StartedAt         time.Time `json:"started_at"`
	StartErr          string    `json:"start_err"`
}

func dailySessionSnapshotPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazyrecall", "daily_session_resume.json"), nil
}

func LoadDailySessionSnapshot() (DailySessionSnapshot, error) {
	path, err := dailySessionSnapshotPath()
	if err != nil {
		return DailySessionSnapshot{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DailySessionSnapshot{}, nil
	}
	if err != nil {
		return DailySessionSnapshot{}, err
	}
	var snapshot DailySessionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return DailySessionSnapshot{}, err
	}
	return snapshot, nil
}

func SaveDailySessionSnapshot(snapshot DailySessionSnapshot) error {
	path, err := dailySessionSnapshotPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func ClearDailySessionSnapshot() error {
	path, err := dailySessionSnapshotPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
