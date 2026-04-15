package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExcludedWords_FileNotExist(t *testing.T) {
	excluded, err := loadExcludedWordsFromPath("/nonexistent/path/exclude.txt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(excluded) != 0 {
		t.Errorf("expected empty map, got %v", excluded)
	}
}

func TestLoadExcludedWords_ParsesCorrectly(t *testing.T) {
	content := "# comment\nthe\nThe\nI\n\n  a  \n"
	tmp := filepath.Join(t.TempDir(), "exclude.txt")
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	excluded, err := loadExcludedWordsFromPath(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, word := range []string{"the", "i", "a"} {
		if !excluded[word] {
			t.Errorf("expected %q to be excluded", word)
		}
	}
	if excluded["# comment"] {
		t.Error("comment line should not be in excluded set")
	}
	if len(excluded) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(excluded), excluded)
	}
}
