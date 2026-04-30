package process

import (
	"fmt"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SummaryArgs struct {
	Item domain.SynthesizedItem `json:"item"`
}

type SummaryResult struct {
	HTML string `json:"html"`
}

func NewSummaryTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_html_summary",
		Description: "Generates a formatted HTML summary for a specific knowledge tree item.",
	}, func(ctx tool.Context, args SummaryArgs) (SummaryResult, error) {
		description := args.Item.Description
		if description == "" {
			description = args.Item.Label
		}
		return SummaryResult{
			HTML: fmt.Sprintf("<p><strong>%s</strong>: %s</p>", base.HtmlEscape(args.Item.Label), base.HtmlEscape(description)),
		}, nil
	})
}
