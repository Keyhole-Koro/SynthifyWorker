package process

import (
	"fmt"
	"strings"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/domain"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SummaryArgs struct {
	Item domain.SynthesizedItem `json:"item"`
}

type SummaryResult struct {
	HTML string `json:"html"`
}

func NewSummaryTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_html_summary",
		Description: "Generates a high-quality, formatted HTML summary for a specific knowledge tree item using the LLM.",
	}, func(ctx tool.Context, args SummaryArgs) (SummaryResult, error) {
		if b == nil || b.LLM == nil {
			return SummaryResult{HTML: args.Item.Content}, nil
		}

		html, _, err := b.LLM.GenerateText(ctx, llm.TextRequest{
			SystemPrompt: `You are a Technical Writer. Convert the provided item details into a professional, rich HTML summary.

Rules:
- NO MARKDOWN: Never use #, **, etc. Use HTML tags only.
- RICH CLASSES: Use available styles to enhance readability:
  - <p class="lede">: for the opening summary.
  - <div class="stat-card">: for key numbers or metrics.
  - <div class="tip-box">: for insights.
  - <table>: for structured data.
- INTERNAL LINKS: Retain all existing <a data-paper-id="..."> links if present in the input.
- Conciseness: Be deep but efficient. 1-4 paragraphs max.`,
			UserPrompt: fmt.Sprintf("Title: %s\nDescription: %s\nRaw Content: %s", args.Item.Title, args.Item.Description, args.Item.Content),
		})

		if err != nil {
			return SummaryResult{HTML: args.Item.Content}, nil
		}

		return SummaryResult{
			HTML: strings.TrimSpace(html),
		}, nil
	})
}
