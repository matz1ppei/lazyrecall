package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ExclusionsPath は除外リストファイルのパスを返す。
func ExclusionsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazyrecall", "exclude.txt"), nil
}

// LoadExcludedWords は exclude.txt を読み込み、小文字キーの集合を返す。
// ファイルが存在しない場合はエラーなしで空マップを返す。
// '#' で始まる行はコメント、空行は無視する。
func LoadExcludedWords() (map[string]bool, error) {
	path, err := ExclusionsPath()
	if err != nil {
		return map[string]bool{}, nil
	}
	return loadExcludedWordsFromPath(path)
}

// AppendExcludedWord は word（小文字化）を exclude.txt に1行追記する。
// ファイル・ディレクトリが存在しない場合は自動作成する。
func AppendExcludedWord(word string) error {
	return setExcludedWordAtPathFromUserConfig(word, true)
}

// SetExcludedWord turns exclusion for word on or off.
func SetExcludedWord(word string, excluded bool) error {
	return setExcludedWordAtPathFromUserConfig(word, excluded)
}

func setExcludedWordAtPathFromUserConfig(word string, excluded bool) error {
	path, err := ExclusionsPath()
	if err != nil {
		return err
	}
	return setExcludedWordAtPath(path, word, excluded)
}

func loadExcludedWordsFromPath(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result[strings.ToLower(line)] = true
	}
	return result, sc.Err()
}

func setExcludedWordAtPath(path, word string, excluded bool) error {
	normalized := strings.ToLower(strings.TrimSpace(word))
	if normalized == "" {
		return nil
	}

	existing, err := loadExcludedWordsFromPath(path)
	if err != nil {
		return err
	}

	if excluded {
		existing[normalized] = true
	} else {
		delete(existing, normalized)
	}

	if len(existing) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	words := make([]string, 0, len(existing))
	for word := range existing {
		words = append(words, word)
	}
	slices.Sort(words)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, word := range words {
		if _, err := fmt.Fprintf(f, "%s\n", word); err != nil {
			return err
		}
	}
	return nil
}
