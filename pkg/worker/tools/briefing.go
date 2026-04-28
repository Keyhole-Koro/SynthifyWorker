package tools

import (
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type BriefArgs struct {
	Outline []string `json:"outline"`
}

type BriefResult struct {
	Brief pipeline.DocumentBrief `json:"brief"`
}

func NewBriefTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_brief",
		Description: "Analyzes the document outline to generate a high-level summary and key themes.",
	}, func(ctx tool.Context, args BriefArgs) (BriefResult, error) {
		return BriefResult{
			Brief: pipeline.DocumentBrief{
				Topic:        "Document Topic Stub",
				ClaimSummary: "Primary Claim Summary Stub",
			},
		}, nil
	})
}
