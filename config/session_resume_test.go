package config

import "testing"

func TestDailySessionSnapshotRoundTripAndClear(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", homeDir+"/.config")

	want := DailySessionSnapshot{
		Date:              "2026-04-24",
		CardIDs:           []int64{3, 1, 2},
		ReviewSessionID:   42,
		Phase:             "match",
		ReviewCorrectIDs:  []int64{3, 1},
		ReverseCorrectIDs: []int64{3},
		MatchWrongIDs:     []int64{2},
		BlankCorrectIDs:   []int64{1, 2},
		BlankSkipped:      false,
		RetryCardIDs:      []int64{2},
		StartErr:          "sample",
	}

	if err := SaveDailySessionSnapshot(want); err != nil {
		t.Fatalf("SaveDailySessionSnapshot: %v", err)
	}

	got, err := LoadDailySessionSnapshot()
	if err != nil {
		t.Fatalf("LoadDailySessionSnapshot: %v", err)
	}
	if got.Date != want.Date || got.Phase != want.Phase || got.ReviewSessionID != want.ReviewSessionID {
		t.Fatalf("loaded snapshot mismatch: got %+v want %+v", got, want)
	}
	if len(got.CardIDs) != 3 || got.CardIDs[0] != 3 || got.MatchWrongIDs[0] != 2 {
		t.Fatalf("loaded snapshot IDs mismatch: got %+v", got)
	}

	if err := ClearDailySessionSnapshot(); err != nil {
		t.Fatalf("ClearDailySessionSnapshot: %v", err)
	}
	got, err = LoadDailySessionSnapshot()
	if err != nil {
		t.Fatalf("LoadDailySessionSnapshot after clear: %v", err)
	}
	if got.Date != "" || len(got.CardIDs) != 0 {
		t.Fatalf("expected empty snapshot after clear, got %+v", got)
	}
}
