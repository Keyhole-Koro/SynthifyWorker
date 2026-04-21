package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type SemanticChunkingStage struct {
	assembler workercontext.Assembler
	llm       workerllm.Client
}

func NewSemanticChunkingStage(assembler workercontext.Assembler, llm workerllm.Client) *SemanticChunkingStage {
	return &SemanticChunkingStage{assembler: assembler, llm: llm}
}

func (s *SemanticChunkingStage) Name() pipeline.StageName { return pipeline.StageSemanticChunking }

func (s *SemanticChunkingStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	bundle := s.assembler.ForChunking(pctx)
	if s.llm != nil {
		chunks, outline, err := s.generateChunks(ctx, bundle, pctx)
		if err != nil {
			return err
		}
		pctx.Chunks = chunks
		pctx.Outline = outline
		return nil
	}

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

func (s *SemanticChunkingStage) generateChunks(ctx context.Context, bundle workercontext.ContextBundle, pctx *pipeline.PipelineContext) ([]pipeline.Chunk, []string, error) {
	resp, err := s.llm.GenerateStructured(ctx, workerllm.StructuredRequest{
		SystemPrompt: bundle.SystemPrompt + "\nSchema version: " + bundle.SchemaVersion + "\nReturn JSON: {\"chunks\":[{\"chunk_index\":0,\"heading\":\"...\",\"text\":\"...\"}]}",
		UserPrompt:   bundle.UserPrompt,
		SourceFiles:  bundle.SourceFiles,
		Schema: map[string]any{
			"type": "object",
		},
	})
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		Chunks []struct {
			ChunkIndex int    `json:"chunk_index"`
			Heading    string `json:"heading"`
			Text       string `json:"text"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, nil, err
	}
	chunks := make([]pipeline.Chunk, 0, len(parsed.Chunks))
	outline := make([]string, 0, len(parsed.Chunks))
	for idx, chunk := range parsed.Chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			continue
		}
		heading := strings.TrimSpace(chunk.Heading)
		if heading == "" {
			heading = fmt.Sprintf("Section %d", idx+1)
		}
		chunks = append(chunks, pipeline.Chunk{
			ChunkIndex: idx,
			Heading:    heading,
			Text:       text,
		})
		outline = append(outline, heading)
	}
	if len(chunks) == 0 {
		return nil, nil, fmt.Errorf("semantic chunking produced no chunks")
	}
	return chunks, outline, nil
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
