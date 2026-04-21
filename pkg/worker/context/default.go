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

func (a *DefaultAssembler) ForPass1(pctx *pipeline.PipelineContext, chunkIdx int) ContextBundle {
	chunkText := ""
	if chunkIdx >= 0 && chunkIdx < len(pctx.Chunks) {
		chunkText = pctx.Chunks[chunkIdx].Text
	}
	return a.buildBundle(workerprompts.Pass1Extraction, chunkText, pctx.SourceFiles, estimateTokens(chunkText))
}

func (a *DefaultAssembler) ForPass2Normal(pctx *pipeline.PipelineContext) ContextBundle {
	userPrompt := strings.Join(pctx.Outline, "\n")
	return a.buildBundle(workerprompts.Pass2Synthesis, userPrompt, nil, estimateTokens(userPrompt)+len(pctx.SynthesizedNodes)*20)
}

func (a *DefaultAssembler) ForPass2Lite(pctx *pipeline.PipelineContext, sectionIdx int) ContextBundle {
	return a.ForPass2Normal(pctx)
}

func (a *DefaultAssembler) ForPass2Final(pctx *pipeline.PipelineContext) ContextBundle {
	return a.ForPass2Normal(pctx)
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
