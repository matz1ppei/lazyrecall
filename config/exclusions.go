package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
	path, err := ExclusionsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", strings.ToLower(word))
	return err
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
