package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

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

func (c *ClaudeClient) GenerateCard(ctx context.Context, topic string) (string, string, string, error) {
	prompt := fmt.Sprintf("Generate a flashcard about: %s. Return JSON {\"front\": ..., \"back\": ..., \"hint\": ...}.", topic)
	raw, err := c.chat(ctx, prompt)
	if err != nil {
		return "", "", "", err
	}

	start := bytes.IndexByte([]byte(raw), '{')
	end := bytes.LastIndexByte([]byte(raw), '}')
	if start < 0 || end < 0 || end < start {
		return "", "", "", fmt.Errorf("claude: no JSON in response: %s", raw)
	}

	var card generatedCard
	if err := json.Unmarshal([]byte(raw[start:end+1]), &card); err != nil {
		return "", "", "", fmt.Errorf("claude: parse JSON: %w", err)
	}
	return card.Front, card.Back, card.Hint, nil
}
