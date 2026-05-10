package base

import (
	"context"
	"encoding/json"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/storage"
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

// JobContext holds metadata about the currently executing LLM job.
type JobContext struct {
	JobID       string
	WorkspaceID string
	DocumentID  string
}

// Context provides shared dependencies to all tools.
type Context struct {
	Repo     Repository
	Embedder Embedder
	LLM      LLMClient
	FS       *storage.FileSystem
	Usage    *UsageLimiter
	Memories []PromptMemory
	Job      *JobContext
}

func (b *Context) BeginJob(ctx context.Context, jobID string, wsID, docID string) {
	if b == nil {
		return
	}
	b.Job = &JobContext{
		JobID:       jobID,
		WorkspaceID: wsID,
		DocumentID:  docID,
	}
	if b.Usage != nil {
		b.Usage.BeginJob(ctx, jobID)
	}
}

func (b *Context) IncrementLLMCalls(ctx context.Context) error {
	if b == nil || b.Usage == nil {
		return nil
	}
	return b.Usage.IncrementLLMCalls(ctx)
}

func (b *Context) IncrementToolRuns(ctx context.Context) error {
	if b == nil || b.Usage == nil {
		return nil
	}
	return b.Usage.IncrementToolRuns(ctx)
}

func (b *Context) IncrementItemCreations(ctx context.Context, count int) error {
	if b == nil || b.Usage == nil {
		return nil
	}
	return b.Usage.IncrementItemCreations(ctx, count)
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
