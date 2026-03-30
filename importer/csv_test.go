package importer

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ippei/poc-anki-claude/db"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func writeCSV(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test*.csv")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestImportCSV_ThreeColumns(t *testing.T) {
	database := openTestDB(t)
	path := writeCSV(t, "front,back,hint\nHello,Hola,greeting\nGoodbye,Adios,farewell\nCat,Gato,animal\n")

	n, err := ImportCSV(database, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("want 3 imported, got %d", n)
	}
}

func TestImportCSV_TwoColumns(t *testing.T) {
	database := openTestDB(t)
	path := writeCSV(t, "front,back\nHello,Hola\nGoodbye,Adios\n")

	n, err := ImportCSV(database, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 imported, got %d", n)
	}
}

func TestImportCSV_SkipsBadRows(t *testing.T) {
	database := openTestDB(t)
	// Row 2 has only 1 column (bad), rows 3 and 4 are good
	path := writeCSV(t, "front,back\nonly_one_column\nGood,Row\nAnother,Good\n")

	n, err := ImportCSV(database, path)
	if err == nil {
		t.Error("expected an error for bad row, got nil")
	}
	if n != 2 {
		t.Errorf("want 2 imported (skipping bad row), got %d", n)
	}
}
