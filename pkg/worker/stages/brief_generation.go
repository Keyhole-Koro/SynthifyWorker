package stages

import (
	"context"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type BriefGenerationStage struct {
	assembler workercontext.Assembler
}

func NewBriefGenerationStage(assembler workercontext.Assembler) *BriefGenerationStage {
	return &BriefGenerationStage{assembler: assembler}
}

func (s *BriefGenerationStage) Name() pipeline.StageName { return pipeline.StageBriefGeneration }

func (s *BriefGenerationStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	_ = s.assembler.ForBriefGeneration(pctx)
	levelHints := append([]string{}, pctx.Outline...)
	brief := &pipeline.DocumentBrief{
		Topic:        firstNonEmpty(append(levelHints, pctx.Filename, "Document")...),
		Level01Hints: uniqueNonEmpty(pctx.Outline),
		ClaimSummary: firstSentence(pctx.RawText),
		Entities:     extractMetrics(pctx.RawText),
		Outline:      append([]string(nil), pctx.Outline...),
	}
	pctx.DocumentBrief = brief
	pctx.SectionBriefs = make([]pipeline.SectionBrief, 0, len(pctx.Chunks))
	for _, chunk := range pctx.Chunks {
		pctx.SectionBriefs = append(pctx.SectionBriefs, pipeline.SectionBrief{
			Heading:         chunk.Heading,
			Topic:           chunk.Heading,
			NodeCandidates:  []string{chunk.Heading},
			ConnectionHints: strings.TrimSpace(firstSentence(chunk.Text)),
		})
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	for _, sep := range []string{"。", ".", "\n"} {
		if idx := strings.Index(text, sep); idx > 0 {
			return strings.TrimSpace(text[:idx+len(sep)])
		}
	}
	return text
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
