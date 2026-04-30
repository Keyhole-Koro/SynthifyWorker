package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID       string         `json:"job_id"`
	DocumentID  string         `json:"document_id"`
	WorkspaceID string         `json:"workspace_id"`
	Chunks      []domain.Chunk `json:"chunks" jsonschema:"description=The specific segments of text to analyze now"`
	Instruction string         `json:"instruction,omitempty" jsonschema:"description=Specific focus or constraints for this synthesis call"`
}

type SynthesisResult struct {
	Items []domain.SynthesizedItem `json:"items"`
}

func NewSynthesisTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "goal_driven_synthesis",
		Description: "Synthesizes a structured knowledge tree from document chunks based on a brief and optional instructions.",
	}, func(ctx tool.Context, args SynthesisArgs) (SynthesisResult, error) {
		items, err := synthesize(ctx, b.LLM, args)
		if err != nil {
			items = deterministicSynthesis(args.DocumentID, args.Chunks)
		}
		return SynthesisResult{Items: items}, nil
	})
}

func synthesize(ctx context.Context, llmClient base.LLMClient, args SynthesisArgs) ([]domain.SynthesizedItem, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llm not configured")
	}

	var sb strings.Builder
	for _, chunk := range args.Chunks {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", chunk.ChunkIndex, chunk.Heading, chunk.Text)
	}

	instruction := args.Instruction
	if instruction == "" {
		instruction = "none"
	}

	type llmOutput struct {
		Items []domain.SynthesizedItem `json:"items"`
	}

	raw, err := llmClient.GenerateStructured(ctx, llm.StructuredRequest{
		SystemPrompt: `You are a knowledge architect. Convert document chunks into a hierarchical knowledge tree.

Rules:
- Use parent_local_id to express parent-child relationships. Root-level items have empty parent_local_id.
- Assign local_id as "item_1", "item_2", etc.
- level: 1 for root-level items, 2 for children, 3 for grandchildren.
- description: concise explanation grounded in the source text. No hallucination.
- summary_html: 1-3 <p> paragraphs. Use <strong> for key terms.
  Link to child items with <a data-paper-id="{local_id}">term</a> so readers can expand them inline.
  You may also use <blockquote> for quotations, <table> for tabular data,
  <div class="compare-grid"><div class="compare-col">...</div><div class="compare-col">...</div></div> for side-by-side comparisons,
  and <div class="callout">...</div> for warnings or key takeaways.
- override_css: optional CSS string to style this item's content. Use only when a custom layout
  genuinely improves readability (e.g. defining .compare-grid or .callout). Leave empty otherwise.
- source_chunk_ids: list of chunk IDs referenced (format: "{document_id}_chunk_{index}").
- The document brief and glossary are in your system context — use them.`,
		UserPrompt: fmt.Sprintf("document_id: %s\nInstruction: %s\n\nChunks:\n%s", args.DocumentID, instruction, sb.String()),
		Schema:     llmOutput{},
	})
	if err != nil {
		return nil, err
	}

	var out llmOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, fmt.Errorf("llm returned no items")
	}
	return out.Items, nil
}

func deterministicSynthesis(documentID string, chunks []domain.Chunk) []domain.SynthesizedItem {
	items := make([]domain.SynthesizedItem, 0, len(chunks))
	for _, chunk := range chunks {
		label := strings.TrimSpace(chunk.Heading)
		if label == "" {
			label = fmt.Sprintf("Section %d", chunk.ChunkIndex+1)
		}
		description := base.SummarizePlainText(chunk.Text, 360)
		items = append(items, domain.SynthesizedItem{
			LocalID:        fmt.Sprintf("chunk_%d", chunk.ChunkIndex),
			Label:          label,
			Level:          1,
			Description:    description,
			SummaryHTML:    "<p>" + base.HtmlEscape(description) + "</p>",
			SourceChunkIDs: []string{fmt.Sprintf("%s_chunk_%d", documentID, chunk.ChunkIndex)},
		})
	}
	return items
}
