package llm

import (
	"context"
	"encoding/json"

	"github.com/synthify/backend/packages/shared/domain"
)

type Client interface {
	GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error)
	GenerateText(ctx context.Context, req TextRequest) (string, error)
}

// ClientWithUsage extends Client with variants that surface LLM token usage.
// Implementations that have access to per-call usage (e.g. Gemini) should implement
// this in addition to Client; lightweight wrappers may skip it. Callers can type-assert:
//
//	if cwu, ok := llm.(ClientWithUsage); ok { /* report usage */ }
type ClientWithUsage interface {
	Client
	GenerateStructuredWithUsage(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error)
	GenerateTextWithUsage(ctx context.Context, req TextRequest) (string, Usage, error)
}

// Usage is the per-call token accounting returned by the LLM provider.
// Model is the canonical name as the provider reports it (e.g. "gemini-3-flash-preview").
type Usage struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
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

// UsageReporter is the worker-side contract for shipping LLM usage to the billing API.
// The concrete implementation is a Connect client backed by the service-token auth path
// (worker → API internal call). Implementations should swallow transient errors and
// surface only invalid-input / auth failures so a metering blip never kills a job.
type UsageReporter interface {
	RecordUsage(ctx context.Context, event domain.UsageEvent) error
}
