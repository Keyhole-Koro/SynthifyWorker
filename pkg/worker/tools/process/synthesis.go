package process

import (
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SynthesisArgs struct {
	JobID       string         `json:"job_id"`
	DocumentID  string         `json:"document_id"`
	WorkspaceID string         `json:"workspace_id"`
	Chunks      []domain.Chunk `json:"chunks" jsonschema:"description=The specific segments of text to analyze now"`
	Instruction string         `json:"instruction,omitempty" jsonschema:"description=Specific focus or constraints for this synthesis call"`
}

type SynthesisResult struct {
	Items []domain.SynthesizedItem `json:"items"`
}

func NewSynthesisTool(b *base.Context) (tool.Tool, error) {
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
			description := base.SummarizePlainText(chunk.Text, 360)
			items = append(items, domain.SynthesizedItem{
				LocalID:        fmt.Sprintf("chunk_%d", chunk.ChunkIndex),
				Label:          label,
				Level:          1,
				Description:    description,
				SummaryHTML:    "<p>" + base.HtmlEscape(description) + "</p>",
				SourceChunkIDs: []string{fmt.Sprintf("%s_chunk_%d", args.DocumentID, chunk.ChunkIndex)},
			})
		}
		return SynthesisResult{Items: items}, nil
	})
}

