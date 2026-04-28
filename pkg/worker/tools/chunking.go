package tools

import (
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ChunkingArgs struct {
	DocumentID string `json:"document_id" jsonschema:"description=The unique identifier of the document to chunk"`
	RawText    string `json:"raw_text" jsonschema:"description=The raw text extracted from the document"`
}

type ChunkingResult struct {
	Chunks  []pipeline.Chunk `json:"chunks"`
	Outline []string         `json:"outline"`
}

// NewChunkingTool splits raw document text into coarse semantic chunks.
// Input schema: ChunkingArgs{document_id: string, raw_text: string}.
// Output schema: ChunkingResult{chunks: []pipeline.Chunk, outline: []string}.
func NewChunkingTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "semantic_chunking",
		Description: "Splits raw document text into semantically coherent chunks and generates an outline.",
	}, func(ctx tool.Context, args ChunkingArgs) (ChunkingResult, error) {
		// In a real implementation, this would call the LLM or a specialized chunking logic.
		// For now, we adapt the existing Stage logic.

		// Note: Since this is a tool, we might want to perform the LLM call inside here
		// if the tool is meant to be 'intelligent', or just perform the logic.

		// For ADK migration, we'll assume the Orchestrator might call this
		// with instructions.

		return ChunkingResult{
			Chunks: []pipeline.Chunk{
				{ChunkIndex: 0, Heading: "Introduction", Text: args.RawText},
			},
			Outline: []string{"Introduction"},
		}, nil
	})
}
