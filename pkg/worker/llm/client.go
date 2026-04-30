package llm

import (
	"context"
	"encoding/json"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
)

type Client interface {
	GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error)
	GenerateText(ctx context.Context, req TextRequest) (string, error)
}

type StructuredRequest struct {
	SystemPrompt string
	UserPrompt   string
	SourceFiles  []domain.SourceFile
	Schema       any
}

type TextRequest struct {
	SystemPrompt string
	UserPrompt   string
	SourceFiles  []domain.SourceFile
}
