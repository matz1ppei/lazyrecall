package dict

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const baseURL = "https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018"

// supportedLangs maps ISO 639-1 code → display name.
var supportedLangs = map[string]string{
	"ar": "Arabic", "cs": "Czech", "da": "Danish", "de": "German",
	"el": "Greek", "en": "English", "es": "Spanish", "fi": "Finnish",
	"fr": "French", "he": "Hebrew", "hi": "Hindi", "hu": "Hungarian",
	"id": "Indonesian", "it": "Italian", "ja": "Japanese", "ko": "Korean",
	"nl": "Dutch", "no": "Norwegian", "pl": "Polish", "pt": "Portuguese",
	"ro": "Romanian", "ru": "Russian", "sv": "Swedish", "th": "Thai",
	"tr": "Turkish", "uk": "Ukrainian", "vi": "Vietnamese", "zh": "Chinese",
}

// topicAliases maps topic keywords (lower-case) → language code.
var topicAliases = map[string]string{
	// English names
	"arabic": "ar", "czech": "cs", "danish": "da", "german": "de",
	"greek": "el", "english": "en", "spanish": "es", "finnish": "fi",
	"french": "fr", "hebrew": "he", "hindi": "hi", "hungarian": "hu",
	"indonesian": "id", "italian": "it", "japanese": "ja", "korean": "ko",
	"dutch": "nl", "norwegian": "no", "polish": "pl", "portuguese": "pt",
	"romanian": "ro", "russian": "ru", "swedish": "sv", "thai": "th",
	"turkish": "tr", "ukrainian": "uk", "vietnamese": "vi", "chinese": "zh",
	// Japanese names
	"アラビア語": "ar", "チェコ語": "cs", "デンマーク語": "da", "ドイツ語": "de",
	"ギリシャ語": "el", "英語": "en", "スペイン語": "es", "フィンランド語": "fi",
	"フランス語": "fr", "ヘブライ語": "he", "ヒンディー語": "hi", "ハンガリー語": "hu",
	"インドネシア語": "id", "イタリア語": "it", "日本語": "ja", "韓国語": "ko",
	"朝鮮語": "ko", "オランダ語": "nl", "ノルウェー語": "no", "ポーランド語": "pl",
	"ポルトガル語": "pt", "ルーマニア語": "ro", "ロシア語": "ru", "スウェーデン語": "sv",
	"タイ語": "th", "トルコ語": "tr", "ウクライナ語": "uk", "ベトナム語": "vi",
	"中国語": "zh",
}

// DetectLang returns the ISO language code and display name if the topic
// matches a known language. Returns ("", "", false) otherwise.
func DetectLang(topic string) (code, name string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(topic))
	// Exact code match ("es", "fr", …)
	if n, exists := supportedLangs[lower]; exists {
		return lower, n, true
	}
	// Keyword search — check both lower-cased topic and original (for Japanese)
	for keyword, code := range topicAliases {
		if strings.Contains(lower, strings.ToLower(keyword)) ||
			strings.Contains(topic, keyword) {
			return code, supportedLangs[code], true
		}
	}
	return "", "", false
}

// GetWords returns words from the frequency dictionary in rank order.
// The dictionary is downloaded on first use and cached locally.
// Returns up to maxWords entries (pass 0 for all available).
func GetWords(langCode string, maxWords int) ([]string, error) {
	path, err := cachePath(langCode)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := download(langCode, path); err != nil {
			// Clean up partial file on failure
			os.Remove(path)
			return nil, fmt.Errorf("download %s dictionary: %w", langCode, err)
		}
	}
	return readWords(path, maxWords)
}

func cachePath(langCode string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "lazyrecall", "dicts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, langCode+"_50k.txt"), nil
}

func download(langCode, dest string) error {
	url := fmt.Sprintf("%s/%s/%s_50k.txt", baseURL, langCode, langCode)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func readWords(path string, maxWords int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var words []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if maxWords > 0 && len(words) >= maxWords {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Format: "word<tab>count" or "word count"
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			words = append(words, fields[0])
		}
	}
	return words, scanner.Err()
}
