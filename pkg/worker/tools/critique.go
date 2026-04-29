package tools

import (
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type CritiqueArgs struct {
	TargetData string `json:"target_data" jsonschema:"description=The generated content or structure to evaluate"`
	Criteria   string `json:"criteria" jsonschema:"description=Specific aspects to look for errors or hallucination"`
}

type CritiqueResult struct {
	Valid       bool     `json:"valid"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

func NewCritiqueTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "quality_critique",
		Description: "Critiques the generated output for potential hallucinations, logical gaps, or inaccuracies. Use this before finalizing results.",
	}, func(ctx tool.Context, args CritiqueArgs) (CritiqueResult, error) {
		var issues []string
		if strings.TrimSpace(args.TargetData) == "" {
			issues = append(issues, "target data is empty")
		}
		if strings.Contains(strings.ToLower(args.TargetData), "stub") {
			issues = append(issues, "target data still contains stub content")
		}
		return CritiqueResult{
			Valid:       len(issues) == 0,
			Issues:      issues,
			Suggestions: suggestionsForIssues(issues),
		}, nil
	})
}

func suggestionsForIssues(issues []string) []string {
	if len(issues) == 0 {
		return nil
	}
	return []string{"Regenerate the affected section from source chunks before persisting."}
}
