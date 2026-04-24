package tools

import (
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID       string                  `json:"job_id"`
	DocumentID  string                  `json:"document_id"`
	WorkspaceID string                  `json:"workspace_id"`
	Chunks      []pipeline.Chunk        `json:"chunks"`
	Brief       *pipeline.DocumentBrief `json:"brief,omitempty"`
	Instruction string                  `json:"instruction,omitempty" jsonschema:"description=Custom instructions on how to structure the tree"`
}

type SynthesisResult struct {
	Items []pipeline.SynthesizedItem `json:"items"`
}

func NewSynthesisTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "goal_driven_synthesis",
		Description: "Synthesizes a structured knowledge tree from document chunks based on a brief and optional instructions.",
	}, func(ctx tool.Context, args SynthesisArgs) (SynthesisResult, error) {
		// Logic to call LLM and build items would go here.
		// For now, returning a stub to demonstrate the tool structure.
		return SynthesisResult{
			Items: []pipeline.SynthesizedItem{
				{LocalID: "doc_root", Label: "Document Root", Level: 0, Description: "Root of the knowledge tree"},
			},
		}, nil
	})
}
