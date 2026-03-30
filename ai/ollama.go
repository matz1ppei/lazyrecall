package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Choices []struct {
		Message ollamaMessage `json:"message"`
	} `json:"choices"`
}

func (c *OllamaClient) chat(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(ollamaChatRequest{
		Model:  c.model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "user", Content: prompt},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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

func (c *OllamaClient) GenerateHint(ctx context.Context, front, back string) (string, error) {
	prompt := fmt.Sprintf("front: %s\nback: %s\nGenerate a short memory hint.", front, back)
	return c.chat(ctx, prompt)
}

type generatedCard struct {
	Front string `json:"front"`
	Back  string `json:"back"`
	Hint  string `json:"hint"`
}

func (c *OllamaClient) GenerateCard(ctx context.Context, topic string) (string, string, string, error) {
	prompt := fmt.Sprintf("Generate a flashcard about: %s. Return JSON {\"front\": ..., \"back\": ..., \"hint\": ...}.", topic)
	raw, err := c.chat(ctx, prompt)
	if err != nil {
		return "", "", "", err
	}

	var card generatedCard
	// Extract JSON from potentially verbose response
	start := bytes.IndexByte([]byte(raw), '{')
	end := bytes.LastIndexByte([]byte(raw), '}')
	if start < 0 || end < 0 || end < start {
		return "", "", "", fmt.Errorf("ollama: no JSON in response: %s", raw)
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &card); err != nil {
		return "", "", "", fmt.Errorf("ollama: parse JSON: %w", err)
	}
	return card.Front, card.Back, card.Hint, nil
}
