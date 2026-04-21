package llm

import (
	"context"
	"encoding/json"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type Client interface {
	GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error)
	GenerateText(ctx context.Context, req TextRequest) (string, error)
}

type StructuredRequest struct {
	SystemPrompt string
	UserPrompt   string
	SourceFiles  []pipeline.SourceFile
	Schema       any
}

type TextRequest struct {
	SystemPrompt string
	UserPrompt   string
	SourceFiles  []pipeline.SourceFile
}
