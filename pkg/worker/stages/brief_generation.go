package stages

import (
	"context"
	"encoding/json"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type BriefGenerationStage struct {
	assembler workercontext.Assembler
	llm       workerllm.Client
}

func NewBriefGenerationStage(assembler workercontext.Assembler, llm workerllm.Client) *BriefGenerationStage {
	return &BriefGenerationStage{assembler: assembler, llm: llm}
}

func (s *BriefGenerationStage) Name() pipeline.StageName { return pipeline.StageBriefGeneration }

func (s *BriefGenerationStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	bundle := s.assembler.ForBriefGeneration(pctx)
	if s.llm != nil {
		if brief, sections, err := s.generateBrief(ctx, bundle); err == nil {
			pctx.DocumentBrief = brief
			pctx.SectionBriefs = sections
			return nil
		}
	}
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

func (s *BriefGenerationStage) generateBrief(ctx context.Context, bundle workercontext.ContextBundle) (*pipeline.DocumentBrief, []pipeline.SectionBrief, error) {
	resp, err := s.llm.GenerateStructured(ctx, workerllm.StructuredRequest{
		SystemPrompt: bundle.SystemPrompt + "\nSchema version: " + bundle.SchemaVersion + "\nReturn JSON with document_brief and section_briefs arrays.",
		UserPrompt:   bundle.UserPrompt,
	})
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		DocumentBrief struct {
			Topic        string   `json:"topic"`
			Level01Hints []string `json:"level01_hints"`
			ClaimSummary string   `json:"claim_summary"`
			Entities     []string `json:"entities"`
			Outline      []string `json:"outline"`
		} `json:"document_brief"`
		SectionBriefs []struct {
			Heading         string   `json:"heading"`
			Topic           string   `json:"topic"`
			NodeCandidates  []string `json:"node_candidates"`
			ConnectionHints string   `json:"connection_hints"`
		} `json:"section_briefs"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, nil, err
	}
	brief := &pipeline.DocumentBrief{
		Topic:        strings.TrimSpace(parsed.DocumentBrief.Topic),
		Level01Hints: uniqueNonEmpty(parsed.DocumentBrief.Level01Hints),
		ClaimSummary: strings.TrimSpace(parsed.DocumentBrief.ClaimSummary),
		Entities:     uniqueNonEmpty(parsed.DocumentBrief.Entities),
		Outline:      uniqueNonEmpty(parsed.DocumentBrief.Outline),
	}
	var sections []pipeline.SectionBrief
	for _, section := range parsed.SectionBriefs {
		heading := strings.TrimSpace(section.Heading)
		topic := strings.TrimSpace(section.Topic)
		if heading == "" && topic == "" {
			continue
		}
		sections = append(sections, pipeline.SectionBrief{
			Heading:         firstNonEmpty(heading, topic),
			Topic:           firstNonEmpty(topic, heading),
			NodeCandidates:  uniqueNonEmpty(section.NodeCandidates),
			ConnectionHints: strings.TrimSpace(section.ConnectionHints),
		})
	}
	return brief, sections, nil
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
