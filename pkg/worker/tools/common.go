package tools

import (
	"context"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"google.golang.org/adk/tool"
)

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

// BaseContext provides shared dependencies to all tools.
type BaseContext struct {
	Repo     Repository
	Embedder Embedder
	Glossary *MemoryGlossary
	Journal  *MemoryJournal
}

// ToolContext wraps ADK tool.Context with our custom dependencies.
type ToolContext struct {
	tool.Context
	Base *BaseContext
}

type GlossaryEntry struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}
