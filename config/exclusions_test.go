package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestSetExcludedWordAtPathAddsAndRemovesWords(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "exclude.txt")
	if err := os.WriteFile(tmp, []byte("banana\napple\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := setExcludedWordAtPath(tmp, "Cherry", true); err != nil {
		t.Fatalf("setExcludedWordAtPath add: %v", err)
	}
	if err := setExcludedWordAtPath(tmp, "APPLE", false); err != nil {
		t.Fatalf("setExcludedWordAtPath remove: %v", err)
	}

	gotBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(gotBytes)
	want := "banana\ncherry\n"
	if got != want {
		t.Fatalf("file contents = %q, want %q", got, want)
	}
}

func TestSetExcludedWordAtPathRemovesFileWhenEmpty(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "exclude.txt")
	if err := os.WriteFile(tmp, []byte("apple\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := setExcludedWordAtPath(tmp, "apple", false); err != nil {
		t.Fatalf("setExcludedWordAtPath remove: %v", err)
	}

	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}

func TestSetExcludedWordUsesUserConfigPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	if err := SetExcludedWord("Apple", true); err != nil {
		t.Fatalf("SetExcludedWord add: %v", err)
	}
	if err := SetExcludedWord("Banana", true); err != nil {
		t.Fatalf("SetExcludedWord second add: %v", err)
	}
	if err := SetExcludedWord("Apple", false); err != nil {
		t.Fatalf("SetExcludedWord remove: %v", err)
	}

	excluded, err := LoadExcludedWords()
	if err != nil {
		t.Fatalf("LoadExcludedWords: %v", err)
	}
	want := map[string]bool{"banana": true}
	if !reflect.DeepEqual(excluded, want) {
		t.Fatalf("excluded = %v, want %v", excluded, want)
	}
}
