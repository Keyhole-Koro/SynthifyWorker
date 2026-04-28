package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SearchArgs struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"description=The workspace to search within"`
	Query       string `json:"query" jsonschema:"description=The question or concept to find"`
	Scope       string `json:"scope" jsonschema:"enum=current_document,all_workspace,description=Whether to search only the current file or everything in the workspace"`
}

type SearchResult struct {
	Results []SearchResultItem `json:"results"`
}

type SearchResultItem struct {
	DocumentID string `json:"document_id"`
	ChunkID    string `json:"chunk_id"`
	Text       string `json:"text"`
	Score      float64 `json:"score"`
}

func NewSearchTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "semantic_search",
		Description: "Performs workspace-wide RAG. Finds relevant information across the current document or all documents in the same workspace.",
	}, func(ctx tool.Context, args SearchArgs) (SearchResult, error) {
		// Stub: in real implementation, this performs vector search on PGVector.
		return SearchResult{Results: []SearchResultItem{}}, nil
	})
}
