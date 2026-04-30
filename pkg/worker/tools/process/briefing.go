package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/worker/pkg/worker/tools/memory"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type BriefArgs struct {
	Outline []string `json:"outline"`
}

type BriefResult struct {
	Brief domain.DocumentBrief `json:"brief"`
}

func NewBriefTool(b *base.Context, mem *memory.Brief) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_brief",
		Description: "Analyzes the document outline to generate a high-level summary and key themes.",
	}, func(ctx tool.Context, args BriefArgs) (BriefResult, error) {
		brief, err := generateBrief(ctx, b.LLM, args.Outline)
		if err != nil {
			brief = fallbackBrief(args.Outline)
		}
		mem.Set(brief)
		return BriefResult{Brief: brief}, nil
	})
}

func generateBrief(ctx context.Context, llmClient base.LLMClient, outline []string) (domain.DocumentBrief, error) {
	if llmClient == nil {
		return domain.DocumentBrief{}, fmt.Errorf("llm not configured")
	}

	var sb strings.Builder
	for i, heading := range outline {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, heading)
	}

	raw, err := llmClient.GenerateStructured(ctx, llm.StructuredRequest{
		SystemPrompt: `You are a document analyst. Given a list of section headings, infer:
- topic: the main subject of the document (short phrase)
- claim_summary: the core argument or purpose in one sentence
- entities: key named concepts, systems, or terms (up to 10)
- level01_hints: suggested top-level labels for a knowledge tree (up to 5)
- outline: the original headings unchanged

Respond only with valid JSON matching the schema.`,
		UserPrompt: "Section headings:\n" + sb.String(),
		Schema:     domain.DocumentBrief{},
	})
	if err != nil {
		return domain.DocumentBrief{}, err
	}

	var brief domain.DocumentBrief
	if err := json.Unmarshal(raw, &brief); err != nil {
		return domain.DocumentBrief{}, err
	}
	brief.Outline = append([]string(nil), outline...)
	return brief, nil
}

func fallbackBrief(outline []string) domain.DocumentBrief {
	topic := "Document"
	if len(outline) > 0 && strings.TrimSpace(outline[0]) != "" {
		topic = strings.TrimSpace(outline[0])
	}
	return domain.DocumentBrief{
		Topic:        topic,
		ClaimSummary: "Document organized around: " + strings.Join(outline, ", "),
		Outline:      append([]string(nil), outline...),
	}
}
