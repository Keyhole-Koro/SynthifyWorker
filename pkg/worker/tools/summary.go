package tools

import (
	"fmt"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SummaryArgs struct {
	Item pipeline.SynthesizedItem `json:"item"`
}

type SummaryResult struct {
	HTML string `json:"html"`
}

func NewSummaryTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_html_summary",
		Description: "Generates a formatted HTML summary for a specific knowledge tree item.",
	}, func(ctx tool.Context, args SummaryArgs) (SummaryResult, error) {
		description := strings.TrimSpace(args.Item.Description)
		if description == "" {
			description = args.Item.Label
		}
		return SummaryResult{
			HTML: fmt.Sprintf("<p><strong>%s</strong>: %s</p>", htmlEscape(args.Item.Label), htmlEscape(description)),
		}, nil
	})
}
