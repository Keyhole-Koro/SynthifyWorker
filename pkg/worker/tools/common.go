package tools

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"google.golang.org/adk/tool"
)

// Repository is the interface for data access required by tools.
type Repository interface {
	GetDocument(id string) (*domain.Document, bool)
	GetDocumentChunks(documentID string) ([]*domain.DocumentChunk, bool)
	SaveDocumentChunks(documentID string, chunks []*domain.DocumentChunk) error
	CreateStructuredItemWithCapability(capability *domain.JobCapability, jobID, documentID, workspaceID, label string, level int, description, summaryHTML, createdBy, parentID string, sourceChunkIDs []string) *domain.Item
	UpdateItemSummaryHTMLWithCapability(capability *domain.JobCapability, jobID, itemID, summaryHTML string) bool
	GetWorkspaceRootItemID(workspaceID string) (string, bool)
}

// BaseContext provides shared dependencies to all tools.
type BaseContext struct {
	Repo Repository
}

// ToolContext wraps ADK tool.Context with our custom dependencies.
type ToolContext struct {
	tool.Context
	Base *BaseContext
}
