package llm

import (
	"context"
	"encoding/json"
)

type Client interface {
	GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error)
	GenerateText(ctx context.Context, req TextRequest) (string, error)
}

type StructuredRequest struct {
	SystemPrompt string
	UserPrompt   string
	FileURIs     []string
	Schema       any
}

type TextRequest struct {
	SystemPrompt string
	UserPrompt   string
	FileURIs     []string
}
