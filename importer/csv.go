package importer

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/ippei/lazyrecall/db"
)

// ImportCSV reads a CSV file with header row (front,back,hint) and inserts each
// row as a card. hint column is optional; rows with only 2 columns get empty hint.
// Row-level errors are skipped and collected; all valid rows are imported.
func ImportCSV(database *sql.DB, path string) (imported int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // allow variable columns

	records, err := r.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("parse csv: %w", err)
	}

	if len(records) == 0 {
		return 0, fmt.Errorf("csv has no header row")
	}

	// Skip header row
	var errs []error
	for i, row := range records[1:] {
		if len(row) < 2 {
			errs = append(errs, fmt.Errorf("row %d: need at least 2 columns", i+2))
			continue
		}
		front := row[0]
		back := row[1]
		hint := ""
		if len(row) >= 3 {
			hint = row[2]
		}
		if front == "" || back == "" {
			errs = append(errs, fmt.Errorf("row %d: front and back must not be empty", i+2))
			continue
		}
		id, insertErr := db.CreateCard(database, front, back, hint, "", "", "")
		if insertErr != nil {
			errs = append(errs, fmt.Errorf("row %d: %w", i+2, insertErr))
			continue
		}
		_, _ = db.GetOrCreateReview(database, id)
		imported++
	}

	if len(errs) > 0 {
		return imported, fmt.Errorf("%d row error(s): %v", len(errs), errs[0])
	}
	return imported, nil
}
