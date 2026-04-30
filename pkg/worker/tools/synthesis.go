package tools

import (
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID       string         `json:"job_id"`
	DocumentID  string         `json:"document_id"`
	WorkspaceID string         `json:"workspace_id"`
	Chunks      []domain.Chunk `json:"chunks" jsonschema:"description=The specific segments of text to analyze now"`

	// Contextual Memory
	DocumentBrief   string          `json:"document_brief" jsonschema:"description=Global blueprint and themes of the entire document"`
	Glossary        []GlossaryEntry `json:"glossary,omitempty" jsonschema:"description=Definitions of specialized terms encountered so far"`
	ParentStructure string          `json:"parent_structure,omitempty" jsonschema:"description=The already established parts of the tree to ensure logical continuity"`

	Instruction string `json:"instruction,omitempty" jsonschema:"description=Specific focus or constraints for this synthesis call"`
}

type SynthesisResult struct {
	Items []domain.SynthesizedItem `json:"items"`
}

// NewSynthesisTool turns chunks and optional brief/instructions into tree items.
// Input schema: SynthesisArgs{job_id: string, document_id: string, workspace_id: string, chunks: []domain.Chunk, brief?: *domain.DocumentBrief, instruction?: string}.
// Output schema: SynthesisResult{items: []domain.SynthesizedItem}.
func NewSynthesisTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "goal_driven_synthesis",
		Description: "Synthesizes a structured knowledge tree from document chunks based on a brief and optional instructions.",
	}, func(ctx tool.Context, args SynthesisArgs) (SynthesisResult, error) {
		items := make([]domain.SynthesizedItem, 0, len(args.Chunks))
		for _, chunk := range args.Chunks {
			label := strings.TrimSpace(chunk.Heading)
			if label == "" {
				label = fmt.Sprintf("Section %d", chunk.ChunkIndex+1)
			}
			description := summarizePlainText(chunk.Text, 360)
			items = append(items, domain.SynthesizedItem{
				LocalID:        fmt.Sprintf("chunk_%d", chunk.ChunkIndex),
				Label:          label,
				Level:          1,
				Description:    description,
				SummaryHTML:    "<p>" + htmlEscape(description) + "</p>",
				SourceChunkIDs: []string{fmt.Sprintf("%s_chunk_%d", args.DocumentID, chunk.ChunkIndex)},
			})
		}
		return SynthesisResult{Items: items}, nil
	})
}

func summarizePlainText(text string, maxRunes int) string {
	compact := strings.Join(strings.Fields(text), " ")
	if compact == "" {
		return ""
	}
	runes := []rune(compact)
	if len(runes) <= maxRunes {
		return compact
	}
	return string(runes[:maxRunes]) + "..."
}

func htmlEscape(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return replacer.Replace(text)
}
