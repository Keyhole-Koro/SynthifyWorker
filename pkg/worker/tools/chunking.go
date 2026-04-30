package tools

import (
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ChunkingArgs struct {
	DocumentID string `json:"document_id" jsonschema:"description=The unique identifier of the document to chunk"`
	RawText    string `json:"raw_text" jsonschema:"description=The raw text extracted from the document"`
}

type ChunkingResult struct {
	Chunks  []domain.Chunk `json:"chunks"`
	Outline []string       `json:"outline"`
}

// NewChunkingTool splits raw document text into coarse semantic chunks.
// Input schema: ChunkingArgs{document_id: string, raw_text: string}.
// Output schema: ChunkingResult{chunks: []domain.Chunk, outline: []string}.
func NewChunkingTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "semantic_chunking",
		Description: "Splits raw document text into semantically coherent chunks and generates an outline.",
	}, func(ctx tool.Context, args ChunkingArgs) (ChunkingResult, error) {
		text := strings.TrimSpace(args.RawText)
		if text == "" {
			return ChunkingResult{}, nil
		}

		sections := pipeline.SplitSections(text)
		chunks := make([]domain.Chunk, 0, len(sections))
		outline := make([]string, 0, len(sections))
		for i, section := range sections {
			heading := section.Heading
			if heading == "" {
				heading = fmt.Sprintf("Section %d", i+1)
			}
			chunks = append(chunks, domain.Chunk{ChunkIndex: i, Heading: heading, Text: section.Text})
			outline = append(outline, heading)
		}
		if base != nil && base.Repo != nil {
			if base.Embedder == nil {
				return ChunkingResult{}, fmt.Errorf("embedder is required: configure GEMINI_API_KEY")
			}
			domainChunks := make([]*domain.DocumentChunk, 0, len(chunks))
			for _, chunk := range chunks {
				vec, err := base.Embedder.EmbedText(ctx, chunk.Heading+" "+chunk.Text)
				if err != nil {
					return ChunkingResult{}, fmt.Errorf("embed chunk %d: %w", chunk.ChunkIndex, err)
				}
				domainChunks = append(domainChunks, &domain.DocumentChunk{
					ChunkID:    fmt.Sprintf("%s_chunk_%d", args.DocumentID, chunk.ChunkIndex),
					DocumentID: args.DocumentID,
					Heading:    chunk.Heading,
					Text:       chunk.Text,
					Embedding:  vec.Slice(),
				})
			}
			if err := base.Repo.SaveDocumentChunks(ctx, args.DocumentID, domainChunks); err != nil {
				return ChunkingResult{}, fmt.Errorf("save chunks: %w", err)
			}
		}
		return ChunkingResult{Chunks: chunks, Outline: outline}, nil
	})
}
