package tools

import (
	"context"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"google.golang.org/adk/tool"
)

// Repository is the interface for data access required by tools.
type Repository interface {
	GetDocument(ctx context.Context, id string) (*domain.Document, bool)
	GetDocumentChunks(ctx context.Context, documentID string) ([]*domain.DocumentChunk, bool)
	GetJobCapability(ctx context.Context, jobID string) (*domain.JobCapability, bool)
	SaveDocumentChunks(ctx context.Context, documentID string, chunks []*domain.DocumentChunk) error
	CreateStructuredItemWithCapability(ctx context.Context, capability *domain.JobCapability, jobID, documentID, workspaceID, label string, level int, description, summaryHTML, createdBy, parentID string, sourceChunkIDs []string) *domain.Item
	UpsertItemSource(ctx context.Context, itemID, documentID, chunkID, sourceText string, confidence float64) error
	UpdateItemSummaryHTMLWithCapability(ctx context.Context, capability *domain.JobCapability, jobID, itemID, summaryHTML string) bool
	GetWorkspaceRootItemID(ctx context.Context, workspaceID string) (string, bool)
	SearchRelatedChunks(ctx context.Context, workspaceID, query string, limit int) ([]*domain.DocumentChunk, error)
	SearchRelatedChunksByVector(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*domain.DocumentChunk, error)
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
