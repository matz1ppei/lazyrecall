package ai

import (
	"context"
	"os"
)

type Client interface {
	GenerateHint(ctx context.Context, front, back string) (string, error)
	GenerateCard(ctx context.Context, topic string) (front, back, hint string, err error)
}

func NewClient() (Client, error) {
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
			model = "qwen2.5:7b"
		}
		return &OllamaClient{baseURL: baseURL, model: model}, nil

	case "claude":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, nil
		}
		return newClaudeClient(key), nil

	default:
		return nil, nil
	}
}
