package base

import (
	"context"
	"encoding/json"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"google.golang.org/adk/tool"
)

// LLMClient is the interface for structured and text generation used by process tools.
type LLMClient interface {
	GenerateStructured(ctx context.Context, req llm.StructuredRequest) (json.RawMessage, error)
	GenerateText(ctx context.Context, req llm.TextRequest) (string, error)
}

// Repository is the interface for data access required by tools.
type Repository interface {
	repository.DocumentRepository
	repository.ItemRepository
	repository.TreeRepository
}

// Embedder generates a vector embedding for a text string.
type Embedder interface {
	EmbedText(ctx context.Context, text string) (pgvector.Vector, error)
}

// PromptMemory is a block of state that is automatically injected into the
// system instruction at the start of every agent turn.
type PromptMemory interface {
	RenderForPrompt() string
}

// Context provides shared dependencies to all tools.
type Context struct {
	Repo     Repository
	Embedder Embedder
	LLM      LLMClient
	Memories []PromptMemory
}

// RenderWorkingMemory concatenates all PromptMemory blocks into a single
// markdown section for injection into the agent's system instruction.
func (b *Context) RenderWorkingMemory() string {
	var sb strings.Builder
	sb.WriteString("## Working Memory\n")
	for _, m := range b.Memories {
		sb.WriteString(m.RenderForPrompt())
	}
	return sb.String()
}

// ToolContext wraps ADK tool.Context with our custom dependencies.
type ToolContext struct {
	tool.Context
	Base *Context
}

// HtmlEscape escapes special HTML characters.
func HtmlEscape(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return replacer.Replace(text)
}

// SummarizePlainText trims and truncates text to maxRunes characters.
func SummarizePlainText(text string, maxRunes int) string {
	compact := strings.Join(strings.Fields(text), " ")
	if compact == "" {
		return ""
	}
	runes := []rune(compact)
	if len(runes) <= maxRunes {
		return compact
	}
	return string(runes[:maxRunes]) + "..."
}
