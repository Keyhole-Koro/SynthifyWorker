package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/domain"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID       string         `json:"job_id"`
	DocumentID  string         `json:"document_id"`
	WorkspaceID string         `json:"workspace_id"`
	Chunks      []domain.Chunk `json:"chunks"`
	Instruction string         `json:"instruction,omitempty"`
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
		SystemPrompt: `You are a Lead Knowledge Architect. Convert document chunks into a high-fidelity, hierarchical knowledge tree.

Rules for "content" (STRICT):
- NO MARKDOWN: Never use #, ##, **, or [text](url). Use HTML tags only.
- RICH HTML: Use a variety of structural tags and CSS classes to make the content "alive":
  - <p class="lede">: for important introductory paragraphs.
  - <p class="eyebrow">: for small, bold labels at the top of sections.
  - <div class="hero-block">: for featured summaries with a visual punch.
  - <div class="callout-grid">: for 2-column comparison or fact grids.
  - <div class="stat-card">: inside a grid or alone, use <strong>Number</strong> <span>Label</span>.
  - <div class="tip-box">: for helpful tips or additional context.
  - <blockquote>: for direct quotes from the source.
  - <table>: for technical data or side-by-side specs.
  - <details><summary>: for technical deep-dives that should be hidden by default.
  - <a data-paper-id="{local_id}">: to link to child items. Use the EXACT local_id.
- COMPOSITION: Combine these elements to create a professional technical report feel.

Rules for Structure:
- Use parent_local_id to express relationships. Root-level items have empty parent_local_id.
- Assign local_id as "item_1", "item_2", etc.
- description: a very short, plain-text summary for list views.
- source_chunk_ids: list of chunk IDs referenced (format: "{document_id}_chunk_{index}").`,
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
		title := strings.TrimSpace(chunk.Heading)
		if title == "" {
			title = fmt.Sprintf("Section %d", chunk.ChunkIndex+1)
		}
		description := base.SummarizePlainText(chunk.Text, 360)
		items = append(items, domain.SynthesizedItem{
			LocalID:        fmt.Sprintf("chunk_%d", chunk.ChunkIndex),
			Title:          title,
			Level:          1,
			Description:    description,
			Content:        "<p>" + base.HtmlEscape(description) + "</p>",
			SourceChunkIDs: []string{fmt.Sprintf("%s_chunk_%d", documentID, chunk.ChunkIndex)},
		})
	}
	return items
}
