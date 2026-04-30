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

type MergeCandidate struct {
	LocalID     string `json:"local_id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type MergeArgs struct {
	Items []MergeCandidate `json:"items" jsonschema:"description=Candidate items that may represent the same concept"`
}

type MergeResult struct {
	MergedID string `json:"merged_id"`
	Reason   string `json:"reason"`
}

func NewMergeTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "deduplicate_and_merge",
		Description: "Merges multiple items into a single canonical item to reduce redundancy in the knowledge tree.",
	}, func(ctx tool.Context, args MergeArgs) (MergeResult, error) {
		if len(args.Items) == 0 {
			return MergeResult{Reason: "no items provided"}, nil
		}
		if len(args.Items) == 1 {
			return MergeResult{MergedID: args.Items[0].LocalID, Reason: "only one candidate"}, nil
		}
		return merge(ctx, b.LLM, args.Items)
	})
}

func merge(ctx context.Context, llmClient base.LLMClient, items []MergeCandidate) (MergeResult, error) {
	if llmClient == nil {
		return MergeResult{MergedID: items[0].LocalID, Reason: "llm not configured, selected first"}, nil
	}

	var sb strings.Builder
	for _, item := range items {
		fmt.Fprintf(&sb, "[%s] %s: %s\n", item.LocalID, item.Label, item.Description)
	}

	type llmResult struct {
		MergedID string `json:"merged_id"`
		Reason   string `json:"reason"`
	}

	raw, err := llmClient.GenerateStructured(ctx, llm.StructuredRequest{
		SystemPrompt: `You are a knowledge deduplication expert.
Given a list of knowledge tree items that may represent the same concept,
select the single most comprehensive and accurate one as the canonical item.
Return its local_id and a brief reason for your choice.`,
		UserPrompt: "Candidate items:\n" + sb.String(),
		Schema:     llmResult{},
	})
	if err != nil {
		return MergeResult{MergedID: items[0].LocalID, Reason: "llm error, selected first"}, nil
	}

	var res llmResult
	if err := json.Unmarshal(raw, &res); err != nil || res.MergedID == "" {
		return MergeResult{MergedID: items[0].LocalID, Reason: "parse error, selected first"}, nil
	}
	return MergeResult{MergedID: res.MergedID, Reason: res.Reason}, nil
}
