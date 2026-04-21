package ai

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
)

// extractJSONArray finds the outermost [...] in raw and returns it ready for
// json.Unmarshal. Used for free-form responses (e.g. Claude) that may include
// surrounding text or markdown fences.
func extractJSONArray(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "```json", "")
	raw = strings.ReplaceAll(raw, "```", "")
	start := bytes.IndexByte([]byte(raw), '[')
	end := bytes.LastIndexByte([]byte(raw), ']')
	if start < 0 || end < 0 || end < start {
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return "", fmt.Errorf("no JSON array in response: %s", preview)
	}
	return raw[start : end+1], nil
}

type GeneratedCard struct {
	Front              string
	Back               string
	Hint               string
	Example            string
	ExampleTranslation string
	ExampleWord        string
}

type WordPair struct {
	Front string
	Back  string
}

type Client interface {
	GenerateHint(ctx context.Context, front, back string) (string, error)
	GenerateExample(ctx context.Context, front, back string) (example, translation, exampleWord string, err error)
	GenerateExampleTranslation(ctx context.Context, front, back, example string) (string, error)
	GenerateCard(ctx context.Context, topic string) (front, back, hint string, err error)
	GenerateCards(ctx context.Context, topic string, start, end int) ([]GeneratedCard, error)
	GenerateWordList(ctx context.Context, topic string, rankStart, rankEnd int, exclude []string) ([]WordPair, error)
	GenerateCardsFromWords(ctx context.Context, words []WordPair) ([]GeneratedCard, error)
	// GenerateCardsForWords generates full cards (back+hint+example) for words
	// whose meanings are unknown — used with frequency dictionary input.
	GenerateCardsForWords(ctx context.Context, topic string, words []string) ([]GeneratedCard, error)
	// EvaluateTranslation checks whether the learner's reconstruction of a sentence
	// from its translation is acceptable, returning a brief explanation.
	// feedbackLang specifies the language for the feedback (e.g. "Japanese"); defaults to English if empty.
	EvaluateTranslation(ctx context.Context, front, back, original, userSentence, feedbackLang string) (feedback string, ok bool, err error)
}

// profilePrefix returns a context line to prepend to prompts when userProfile is set.
func profilePrefix(userProfile string) string {
	if userProfile == "" {
		return ""
	}
	return "User context: " + userProfile + "\n"
}

func NewClient(userProfile string) (Client, error) {
	backend := os.Getenv("AI_BACKEND")
	if backend == "" {
		backend = "ollama"
	}

	switch backend {
	case "ollama":
		baseURL := os.Getenv("OLLAMA_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			model = "gemma4:e4b"
		}
		return &OllamaClient{baseURL: baseURL, model: model, userProfile: userProfile}, nil

	case "claude":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, nil
		}
		return newClaudeClient(key, userProfile), nil

	default:
		return nil, nil
	}
}
