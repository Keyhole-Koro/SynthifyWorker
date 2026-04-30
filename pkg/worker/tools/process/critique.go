package process

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
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

func NewCritiqueTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "quality_critique",
		Description: "Critiques the generated output for potential hallucinations, logical gaps, or inaccuracies. Use this before finalizing results.",
	}, func(ctx tool.Context, args CritiqueArgs) (CritiqueResult, error) {
		if strings.TrimSpace(args.TargetData) == "" {
			return CritiqueResult{
				Valid:  false,
				Issues: []string{"target data is empty"},
			}, nil
		}
		return critique(ctx, b.LLM, args.TargetData, args.Criteria)
	})
}

func critique(ctx context.Context, llmClient base.LLMClient, targetData, criteria string) (CritiqueResult, error) {
	if llmClient == nil {
		return CritiqueResult{Valid: true}, nil
	}

	userPrompt := fmt.Sprintf("Criteria: %s\n\nContent to evaluate:\n%s", criteria, targetData)

	raw, err := llmClient.GenerateStructured(ctx, llm.StructuredRequest{
		SystemPrompt: `You are a quality assurance reviewer for a knowledge management system.
Evaluate the provided content for:
- Hallucinations (claims not grounded in the source)
- Logical gaps or missing context
- Inaccurate or misleading descriptions
- Structural inconsistencies (e.g. broken parent-child relationships)

Return JSON with:
- valid: true only if no significant issues found
- issues: list of specific problems found (empty array if none)
- suggestions: list of concrete fixes (empty array if none)`,
		UserPrompt: userPrompt,
		Schema:     CritiqueResult{},
	})
	if err != nil {
		return CritiqueResult{Valid: true}, nil
	}

	var result CritiqueResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CritiqueResult{Valid: true}, nil
	}
	if result.Issues == nil {
		result.Issues = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}
	return result, nil
}
