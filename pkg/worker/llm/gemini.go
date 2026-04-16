package llm

import (
	"context"
	"encoding/json"
	"errors"
)

type GeminiClient struct {
	Model string
}

func NewGeminiClient(model string) *GeminiClient {
	if model == "" {
		model = "gemini-3.0-flash"
	}
	return &GeminiClient{Model: model}
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error) {
	return nil, errors.New("gemini client is not configured in this local build")
}

func (c *GeminiClient) GenerateText(ctx context.Context, req TextRequest) (string, error) {
	return "", errors.New("gemini client is not configured in this local build")
}
