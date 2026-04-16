package stages

import (
	"context"
	"fmt"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type SemanticChunkingStage struct {
	assembler workercontext.Assembler
}

func NewSemanticChunkingStage(assembler workercontext.Assembler) *SemanticChunkingStage {
	return &SemanticChunkingStage{assembler: assembler}
}

func (s *SemanticChunkingStage) Name() pipeline.StageName { return pipeline.StageSemanticChunking }

func (s *SemanticChunkingStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	_ = s.assembler.ForChunking(pctx)
	sections := splitSections(pctx.RawText)
	var chunks []pipeline.Chunk
	var outline []string
	for idx, section := range sections {
		chunk := pipeline.Chunk{
			ChunkIndex: idx,
			Heading:    section.heading,
			Text:       section.text,
		}
		chunks = append(chunks, chunk)
		outline = append(outline, section.heading)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("semantic chunking produced no chunks")
	}
	pctx.Chunks = chunks
	pctx.Outline = outline
	return nil
}

type section struct {
	heading string
	text    string
}

func splitSections(raw string) []section {
	lines := strings.Split(raw, "\n")
	var sections []section
	currentHeading := "Document"
	var current []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(current, "\n"))
		if text == "" {
			return
		}
		sections = append(sections, section{heading: currentHeading, text: text})
		current = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(strings.Join(current, "")) > 1800 {
				flush()
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "：") {
			flush()
			currentHeading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		current = append(current, trimmed)
		if len([]rune(strings.Join(current, "\n"))) > 1800 {
			flush()
		}
	}
	flush()
	if len(sections) == 0 && strings.TrimSpace(raw) != "" {
		sections = append(sections, section{heading: "Document", text: strings.TrimSpace(raw)})
	}
	return sections
}
