package tools

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
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
		text := strings.TrimSpace(args.RawText)
		if text == "" {
			return ChunkingResult{}, nil
		}

		sections := splitSections(text)
		chunks := make([]pipeline.Chunk, 0, len(sections))
		outline := make([]string, 0, len(sections))
		for i, section := range sections {
			heading := section.heading
			if heading == "" {
				heading = fmt.Sprintf("Section %d", i+1)
			}
			chunks = append(chunks, pipeline.Chunk{ChunkIndex: i, Heading: heading, Text: section.text})
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

type textSection struct {
	heading string
	text    string
}

func splitSections(text string) []textSection {
	const maxRunes = 3500
	var sections []textSection
	var currentHeading string
	var current strings.Builder

	flush := func() {
		body := strings.TrimSpace(current.String())
		if body == "" {
			return
		}
		sections = append(sections, textSection{heading: currentHeading, text: body})
		current.Reset()
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if looksLikeHeading(trimmed) && current.Len() > 0 {
			flush()
			currentHeading = normalizeHeading(trimmed)
		}
		if currentHeading == "" && looksLikeHeading(trimmed) {
			currentHeading = normalizeHeading(trimmed)
		}
		if utf8.RuneCountInString(current.String())+utf8.RuneCountInString(line) > maxRunes {
			flush()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	flush()
	if len(sections) == 0 {
		sections = append(sections, textSection{heading: "Introduction", text: text})
	}
	return sections
}

func looksLikeHeading(line string) bool {
	if line == "" || len([]rune(line)) > 120 {
		return false
	}
	if strings.HasPrefix(line, "#") {
		return true
	}
	if numberedHeadingPattern.MatchString(line) {
		return true
	}
	return strings.HasSuffix(line, ":") && len(strings.Fields(line)) <= 8
}

func normalizeHeading(line string) string {
	return strings.Trim(strings.TrimSpace(line), "#: ")
}
