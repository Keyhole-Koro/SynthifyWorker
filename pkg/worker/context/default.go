package workercontext

import (
	"fmt"
	"os"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	workerprompts "github.com/synthify/backend/worker/pkg/worker/prompts"
)

type DefaultAssembler struct {
	promptDir string
}

func NewDefaultAssembler(promptDir string) *DefaultAssembler {
	return &DefaultAssembler{promptDir: promptDir}
}

func (a *DefaultAssembler) ForChunking(pctx *pipeline.PipelineContext) ContextBundle {
	return a.buildBundle(workerprompts.SemanticChunking, pctx.RawText, pctx.SourceFiles, estimateTokens(pctx.RawText))
}

func (a *DefaultAssembler) ForBriefGeneration(pctx *pipeline.PipelineContext) ContextBundle {
	return a.buildBundle(workerprompts.BriefGeneration, strings.Join(pctx.Outline, "\n"), nil, estimateTokens(pctx.RawText))
}

func (a *DefaultAssembler) ForHTMLSummary(pctx *pipeline.PipelineContext, nodeLocalID string) ContextBundle {
	return a.buildBundle(workerprompts.HTMLSummary, fmt.Sprintf("node=%s", nodeLocalID), nil, 128)
}

func (a *DefaultAssembler) buildBundle(promptName, userPrompt string, sourceFiles []pipeline.SourceFile, tokenEstimate int) ContextBundle {
	spec := workerprompts.MustLookup(promptName)
	return ContextBundle{
		SystemPrompt:  a.readPrompt(promptName),
		UserPrompt:    userPrompt,
		SourceFiles:   append([]pipeline.SourceFile(nil), sourceFiles...),
		TokenEstimate: tokenEstimate,
		PromptName:    spec.Name,
		PromptVersion: spec.PromptVersion,
		SchemaVersion: spec.SchemaVersion,
	}
}

func (a *DefaultAssembler) readPrompt(name string) string {
	path := workerprompts.Path(a.promptDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text)) / 2
}
