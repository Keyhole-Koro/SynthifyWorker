package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Keyhole-Koro/SynthifyShared/config"
	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
}

func NewGeminiClient(cfg config.LLM) *GeminiClient {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		// NewClient during construction might be risky if API key is missing, 
		// but let's follow the pattern.
		return &GeminiClient{model: cfg.GeminiModel}
	}

	return &GeminiClient{
		client: client,
		model:  cfg.GeminiModel,
	}
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error) {
	if c.client == nil {
		return nil, fmt.Errorf("gemini client not initialized")
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		},
		Temperature:      ptr(float64(0.2)),
		ResponseMimeType: "application/json",
	}

	res, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(req.UserPrompt), config)
	if err != nil {
		return nil, fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini api: empty response")
	}

	text := res.Candidates[0].Content.Parts[0].Text
	return json.RawMessage(RepairJSON(text)), nil
}

func (c *GeminiClient) GenerateText(ctx context.Context, req TextRequest) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("gemini client not initialized")
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		},
		Temperature: ptr(float64(0.2)),
	}

	res, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(req.UserPrompt), config)
	if err != nil {
		return "", fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini api: empty response")
	}

	return res.Candidates[0].Content.Parts[0].Text, nil
}

func ptr[T any](v T) *T {
	return &v
}
