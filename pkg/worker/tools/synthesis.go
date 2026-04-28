package tools

import (
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID           string                    `json:"job_id"`
	DocumentID      string                    `json:"document_id"`
	WorkspaceID     string                    `json:"workspace_id"`
	Chunks          []pipeline.Chunk          `json:"chunks" jsonschema:"description=The specific segments of text to analyze now"`

	// Contextual Memory
	DocumentBrief   string                    `json:"document_brief" jsonschema:"description=Global blueprint and themes of the entire document"`
	Glossary        []GlossaryEntry           `json:"glossary,omitempty" jsonschema:"description=Definitions of specialized terms encountered so far"`
	ParentStructure string                    `json:"parent_structure,omitempty" jsonschema:"description=The already established parts of the tree to ensure logical continuity"`

	Instruction     string                    `json:"instruction,omitempty" jsonschema:"description=Specific focus or constraints for this synthesis call"`
}


type SynthesisResult struct {
	Items []pipeline.SynthesizedItem `json:"items"`
}

// NewSynthesisTool turns chunks and optional brief/instructions into tree items.
// Input schema: SynthesisArgs{job_id: string, document_id: string, workspace_id: string, chunks: []pipeline.Chunk, brief?: *pipeline.DocumentBrief, instruction?: string}.
// Output schema: SynthesisResult{items: []pipeline.SynthesizedItem}.
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
