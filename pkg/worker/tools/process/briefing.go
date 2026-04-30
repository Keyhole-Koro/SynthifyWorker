package process

import (
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/tools/memory"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type BriefArgs struct {
	Outline []string `json:"outline"`
}

type BriefResult struct {
	Brief domain.DocumentBrief `json:"brief"`
}

func NewBriefTool(b *memory.Brief) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "generate_brief",
		Description: "Analyzes the document outline to generate a high-level summary and key themes.",
	}, func(ctx tool.Context, args BriefArgs) (BriefResult, error) {
		topic := "Document"
		if len(args.Outline) > 0 && strings.TrimSpace(args.Outline[0]) != "" {
			topic = strings.TrimSpace(args.Outline[0])
		}
		brief := domain.DocumentBrief{
			Topic:        topic,
			ClaimSummary: "Document organized around: " + strings.Join(args.Outline, ", "),
			Outline:      append([]string(nil), args.Outline...),
		}
		b.Set(brief)
		return BriefResult{Brief: brief}, nil
	})
}
