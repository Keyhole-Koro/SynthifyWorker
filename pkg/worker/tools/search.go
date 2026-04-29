package tools

import (
	"fmt"

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
	DocumentID string  `json:"document_id"`
	ChunkID    string  `json:"chunk_id"`
	Text       string  `json:"text"`
	Score      float64 `json:"score"`
}

func NewSearchTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "semantic_search",
		Description: "Performs workspace-wide RAG. Finds relevant information across the current document or all documents in the same workspace.",
	}, func(ctx tool.Context, args SearchArgs) (SearchResult, error) {
		if base == nil || base.Repo == nil {
			return SearchResult{}, fmt.Errorf("repository is not configured")
		}
		if base.Embedder == nil {
			return SearchResult{}, fmt.Errorf("embedder is required: configure GEMINI_API_KEY")
		}

		vec, err := base.Embedder.EmbedText(ctx, args.Query)
		if err != nil {
			return SearchResult{}, fmt.Errorf("embed query: %w", err)
		}
		chunks, err := base.Repo.SearchRelatedChunksByVector(ctx, args.WorkspaceID, vec.Slice(), 8)
		if err != nil {
			return SearchResult{}, fmt.Errorf("vector search: %w", err)
		}
		results := make([]SearchResultItem, 0, len(chunks))
		for i, chunk := range chunks {
			score := 1.0 - float64(i)/float64(len(chunks)+1)
			results = append(results, SearchResultItem{
				DocumentID: chunk.DocumentID,
				ChunkID:    chunk.ChunkID,
				Text:       chunk.Text,
				Score:      score,
			})
		}
		return SearchResult{Results: results}, nil
	})
}
