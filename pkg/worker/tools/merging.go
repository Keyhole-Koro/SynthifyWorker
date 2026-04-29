package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type MergeArgs struct {
	ItemIDs []string `json:"item_ids" jsonschema:"description=List of item IDs that seem to represent the same concept"`
}

type MergeResult struct {
	MergedID string `json:"merged_id"`
	Message  string `json:"message"`
}

func NewMergeTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "deduplicate_and_merge",
		Description: "Merges multiple items into a single canonical item to reduce redundancy in the knowledge tree.",
	}, func(ctx tool.Context, args MergeArgs) (MergeResult, error) {
		if len(args.ItemIDs) == 0 {
			return MergeResult{Message: "No items provided"}, nil
		}
		return MergeResult{MergedID: args.ItemIDs[0], Message: "Selected canonical item for duplicate group"}, nil
	})
}
