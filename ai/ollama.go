package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaClient struct {
	baseURL string
	model   string
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model          string            `json:"model"`
	Messages       []ollamaMessage   `json:"messages"`
	Stream         bool              `json:"stream"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type ollamaChatResponse struct {
	Choices []struct {
		Message ollamaMessage `json:"message"`
	} `json:"choices"`
}

func (c *OllamaClient) do(ctx context.Context, req ollamaChatRequest) (string, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: status %d: %s", resp.StatusCode, b)
	}

	var result ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama: empty response")
	}
	return result.Choices[0].Message.Content, nil
}

// chat sends a free-form prompt.
func (c *OllamaClient) chat(ctx context.Context, prompt string) (string, error) {
	return c.do(ctx, ollamaChatRequest{
		Model:  c.model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "user", Content: prompt},
		},
	})
}

// chatJSON sends a prompt with response_format=json_object, guaranteeing valid JSON output.
func (c *OllamaClient) chatJSON(ctx context.Context, prompt string) (string, error) {
	return c.do(ctx, ollamaChatRequest{
		Model:  c.model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "user", Content: prompt},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

func (c *OllamaClient) GenerateHint(ctx context.Context, front, back string) (string, error) {
	prompt := fmt.Sprintf("front: %s\nback: %s\nGenerate a short memory hint.", front, back)
	return c.chat(ctx, prompt)
}

func (c *OllamaClient) GenerateExample(ctx context.Context, front, back string) (example, translation string, err error) {
	prompt := fmt.Sprintf(
		"word: %s\nmeaning: %s\nGenerate one natural example sentence using this word, and its English translation.\nReturn a JSON object: {\"example\": \"...\", \"translation\": \"...\"}",
		front, back,
	)
	raw, err := c.chatJSON(ctx, prompt)
	if err != nil {
		return "", "", err
	}
	var result struct {
		Example     string `json:"example"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", "", fmt.Errorf("ollama: parse example JSON: %w (raw: %.200s)", err, raw)
	}
	return result.Example, result.Translation, nil
}

func (c *OllamaClient) GenerateExampleTranslation(ctx context.Context, front, back, example string) (string, error) {
	prompt := fmt.Sprintf(
		"word: %s\nmeaning: %s\nexample sentence: %s\nTranslate the example sentence to English. Return only the translation.",
		front, back, example,
	)
	return c.chat(ctx, prompt)
}

type generatedCard struct {
	Front              string `json:"front"`
	Back               string `json:"back"`
	Hint               string `json:"hint"`
	Example            string `json:"example"`
	ExampleTranslation string `json:"example_translation"`
}

func (c *OllamaClient) GenerateCard(ctx context.Context, topic string) (string, string, string, error) {
	cards, err := c.GenerateCards(ctx, topic, 1, 1)
	if err != nil || len(cards) == 0 {
		return "", "", "", err
	}
	return cards[0].Front, cards[0].Back, cards[0].Hint, nil
}

func (c *OllamaClient) GenerateCards(ctx context.Context, topic string, rankStart, rankEnd int) ([]GeneratedCard, error) {
	count := rankEnd - rankStart + 1
	prompt := fmt.Sprintf(
		"Generate %d flashcard(s) about: %s. These should be the most common words ranked #%d to #%d by frequency. Return ONLY a JSON array: [{\"front\": ..., \"back\": ..., \"hint\": ..., \"example\": \"example sentence using the word\"}, ...].",
		count, topic, rankStart, rankEnd,
	)
	raw, err := c.chat(ctx, prompt)
	if err != nil {
		return nil, err
	}
	jsonStr, err := extractJSONArray(raw)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	var cards []generatedCard
	if err := json.Unmarshal([]byte(jsonStr), &cards); err != nil {
		return nil, fmt.Errorf("ollama: parse JSON: %w", err)
	}
	result := make([]GeneratedCard, len(cards))
	for i, c := range cards {
		result[i] = GeneratedCard{Front: c.Front, Back: c.Back, Hint: c.Hint, Example: c.Example, ExampleTranslation: c.ExampleTranslation}
	}
	return result, nil
}

func (c *OllamaClient) GenerateWordList(ctx context.Context, topic string, rankStart, rankEnd int, exclude []string) ([]WordPair, error) {
	count := rankEnd - rankStart + 1
	exclusion := ""
	if len(exclude) > 0 {
		exclusion = fmt.Sprintf(" Do not include any of these already-generated words: %s.", strings.Join(exclude, ", "))
	}
	prompt := fmt.Sprintf(
		`List %d of the most common words/phrases for: %s. These should be ranked #%d to #%d by frequency.%s `+
			`Return a JSON object: {"items": [{"front": "<word>", "back": "<meaning/translation>"}, ...]}`,
		count, topic, rankStart, rankEnd, exclusion,
	)
	raw, err := c.chatJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Items []struct {
			Front string `json:"front"`
			Back  string `json:"back"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("ollama: parse JSON: %w", err)
	}
	result := make([]WordPair, len(wrapper.Items))
	for i, p := range wrapper.Items {
		result[i] = WordPair{Front: p.Front, Back: p.Back}
	}
	return result, nil
}

func (c *OllamaClient) GenerateCardsForWords(ctx context.Context, topic string, words []string) ([]GeneratedCard, error) {
	wordsJSON, _ := json.Marshal(words)
	prompt := fmt.Sprintf(
		`For each %s word listed, generate: "back" (English translation/meaning), "hint" (short memory mnemonic), "example" (one natural example sentence that contains the word in its exact listed form), "example_translation" (English translation of the example sentence). `+
			`Important: the example sentence must use the word exactly as listed — do not conjugate or inflect it. `+
			`Words: %s. Return a JSON object: {"items": [{"front": "<word>", "back": "<translation>", "hint": "<mnemonic>", "example": "<sentence>", "example_translation": "<English translation of example>"}, ...]}`,
		topic, string(wordsJSON),
	)
	raw, err := c.chatJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("ollama: parse JSON: %w (raw: %.300s)", err, raw)
	}
	var cards []generatedCard
	if err := json.Unmarshal(wrapper.Items, &cards); err != nil {
		return nil, fmt.Errorf("ollama: parse items: %w (items: %.300s)", err, string(wrapper.Items))
	}
	if len(cards) > len(words) {
		cards = cards[:len(words)]
	}
	result := make([]GeneratedCard, len(cards))
	for i, c := range cards {
		result[i] = GeneratedCard{Front: c.Front, Back: c.Back, Hint: c.Hint, Example: c.Example, ExampleTranslation: c.ExampleTranslation}
	}
	return result, nil
}

func (c *OllamaClient) GenerateCardsFromWords(ctx context.Context, words []WordPair) ([]GeneratedCard, error) {
	wordsJSON, _ := json.Marshal(words)
	prompt := fmt.Sprintf(
		`For each word pair, add a short memory hint, one natural example sentence, and an English translation of that example sentence. Words: %s. `+
			`Return a JSON object: {"items": [{"front": ..., "back": ..., "hint": "short mnemonic", "example": "example sentence", "example_translation": "English translation of example"}, ...]}`,
		string(wordsJSON),
	)
	raw, err := c.chatJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Items []generatedCard `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("ollama: parse JSON: %w", err)
	}
	result := make([]GeneratedCard, len(wrapper.Items))
	for i, c := range wrapper.Items {
		result[i] = GeneratedCard{Front: c.Front, Back: c.Back, Hint: c.Hint, Example: c.Example, ExampleTranslation: c.ExampleTranslation}
	}
	return result, nil
}
