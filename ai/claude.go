package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type ClaudeClient struct {
	client anthropic.Client
}

func newClaudeClient(apiKey string) *ClaudeClient {
	return &ClaudeClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

func (c *ClaudeClient) chat(ctx context.Context, prompt string) (string, error) {
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: 512,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}
	if len(msg.Content) == 0 {
		return "", fmt.Errorf("claude: empty response")
	}
	return msg.Content[0].Text, nil
}

func (c *ClaudeClient) GenerateHint(ctx context.Context, front, back string) (string, error) {
	prompt := fmt.Sprintf("front: %s\nback: %s\nGenerate a short memory hint.", front, back)
	return c.chat(ctx, prompt)
}

func (c *ClaudeClient) GenerateExample(ctx context.Context, front, back string) (example, translation string, err error) {
	prompt := fmt.Sprintf(
		"word: %s\nmeaning: %s\nGenerate one natural example sentence using this word, and its English translation.\nReturn ONLY a JSON object: {\"example\": \"...\", \"translation\": \"...\"}",
		front, back,
	)
	raw, err := c.chat(ctx, prompt)
	if err != nil {
		return "", "", err
	}
	raw = strings.ReplaceAll(raw, "```json", "")
	raw = strings.ReplaceAll(raw, "```", "")
	var result struct {
		Example     string `json:"example"`
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil {
		return "", "", fmt.Errorf("claude: parse example JSON: %w (raw: %.200s)", err, raw)
	}
	return result.Example, result.Translation, nil
}

func (c *ClaudeClient) GenerateExampleTranslation(ctx context.Context, front, back, example string) (string, error) {
	prompt := fmt.Sprintf(
		"word: %s\nmeaning: %s\nexample sentence: %s\nTranslate the example sentence to English. Return only the translation.",
		front, back, example,
	)
	return c.chat(ctx, prompt)
}

func (c *ClaudeClient) GenerateCard(ctx context.Context, topic string) (string, string, string, error) {
	cards, err := c.GenerateCards(ctx, topic, 1, 1)
	if err != nil || len(cards) == 0 {
		return "", "", "", err
	}
	return cards[0].Front, cards[0].Back, cards[0].Hint, nil
}

func (c *ClaudeClient) GenerateCards(ctx context.Context, topic string, rankStart, rankEnd int) ([]GeneratedCard, error) {
	count := rankEnd - rankStart + 1
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: int64(300 * count),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf(
				"Generate %d flashcard(s) about: %s. These should be the most common words ranked #%d to #%d by frequency. Return ONLY a JSON array: [{\"front\": ..., \"back\": ..., \"hint\": ..., \"example\": \"example sentence using the word\"}, ...].",
				count, topic, rankStart, rankEnd,
			))),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude: empty response")
	}
	raw := msg.Content[0].Text
	jsonStr, err := extractJSONArray(raw)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}

	var cards []generatedCard
	if err := json.Unmarshal([]byte(jsonStr), &cards); err != nil {
		return nil, fmt.Errorf("claude: parse JSON: %w", err)
	}

	result := make([]GeneratedCard, len(cards))
	for i, c := range cards {
		result[i] = GeneratedCard{Front: c.Front, Back: c.Back, Hint: c.Hint, Example: c.Example, ExampleTranslation: c.ExampleTranslation}
	}
	return result, nil
}

func (c *ClaudeClient) GenerateWordList(ctx context.Context, topic string, rankStart, rankEnd int, exclude []string) ([]WordPair, error) {
	count := rankEnd - rankStart + 1
	exclusion := ""
	if len(exclude) > 0 {
		exclusion = fmt.Sprintf(" Do not include any of these already-generated words: %s.", strings.Join(exclude, ", "))
	}
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: int64(50 * count),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf(
				"List %d of the most common words/phrases for: %s. These should be ranked #%d to #%d by frequency.%s Return ONLY a JSON array: [{\"front\": \"<word>\", \"back\": \"<meaning/translation>\"}, ...]. No explanations.",
				count, topic, rankStart, rankEnd, exclusion,
			))),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude: empty response")
	}
	raw := msg.Content[0].Text
	jsonStr, err := extractJSONArray(raw)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	var pairs []struct {
		Front string `json:"front"`
		Back  string `json:"back"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &pairs); err != nil {
		return nil, fmt.Errorf("claude: parse JSON: %w", err)
	}
	result := make([]WordPair, len(pairs))
	for i, p := range pairs {
		result[i] = WordPair{Front: p.Front, Back: p.Back}
	}
	return result, nil
}

func (c *ClaudeClient) GenerateCardsForWords(ctx context.Context, topic string, words []string) ([]GeneratedCard, error) {
	wordsJSON, _ := json.Marshal(words)
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: int64(300 * len(words)),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf(
				`For each %s word listed, generate: "back" (English translation/meaning), "hint" (short memory mnemonic), "example" (one natural example sentence), "example_translation" (English translation of the example sentence). ` +
					`Words: %s. Return ONLY a JSON array: [{"front": "<word>", "back": "<translation>", "hint": "<mnemonic>", "example": "<sentence>", "example_translation": "<English translation of example>"}, ...]`,
				topic, string(wordsJSON),
			))),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude: empty response")
	}
	jsonStr, err := extractJSONArray(msg.Content[0].Text)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	var cards []generatedCard
	if err := json.Unmarshal([]byte(jsonStr), &cards); err != nil {
		return nil, fmt.Errorf("claude: parse JSON: %w", err)
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

func (c *ClaudeClient) GenerateCardsFromWords(ctx context.Context, words []WordPair) ([]GeneratedCard, error) {
	wordsJSON, _ := json.Marshal(words)
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: int64(300 * len(words)),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf(
				"For each word pair, add a short memory hint, one natural example sentence, and an English translation of that example sentence. Words: %s. Return ONLY a JSON array (same order): [{\"front\": ..., \"back\": ..., \"hint\": \"short mnemonic\", \"example\": \"example sentence\", \"example_translation\": \"English translation of example\"}, ...].",
				string(wordsJSON),
			))),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(msg.Content) == 0 {
		return nil, fmt.Errorf("claude: empty response")
	}
	raw := msg.Content[0].Text
	jsonStr, err := extractJSONArray(raw)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	var cards []generatedCard
	if err := json.Unmarshal([]byte(jsonStr), &cards); err != nil {
		return nil, fmt.Errorf("claude: parse JSON: %w", err)
	}
	result := make([]GeneratedCard, len(cards))
	for i, c := range cards {
		result[i] = GeneratedCard{Front: c.Front, Back: c.Back, Hint: c.Hint, Example: c.Example, ExampleTranslation: c.ExampleTranslation}
	}
	return result, nil
}
