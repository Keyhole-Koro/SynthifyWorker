package workercontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type DefaultAssembler struct {
	promptDir string
}

func NewDefaultAssembler(promptDir string) *DefaultAssembler {
	return &DefaultAssembler{promptDir: promptDir}
}

func (a *DefaultAssembler) ForChunking(pctx *pipeline.PipelineContext) ContextBundle {
	return ContextBundle{
		SystemPrompt:  a.readPrompt("semantic_chunking"),
		UserPrompt:    pctx.RawText,
		FileURIs:      []string{pctx.FileURI},
		TokenEstimate: estimateTokens(pctx.RawText),
	}
}

func (a *DefaultAssembler) ForBriefGeneration(pctx *pipeline.PipelineContext) ContextBundle {
	return ContextBundle{
		SystemPrompt:  "Generate a high-level brief grounded in the source text.",
		UserPrompt:    strings.Join(pctx.Outline, "\n"),
		TokenEstimate: estimateTokens(pctx.RawText),
	}
}

func (a *DefaultAssembler) ForPass1(pctx *pipeline.PipelineContext, chunkIdx int) ContextBundle {
	chunkText := ""
	if chunkIdx >= 0 && chunkIdx < len(pctx.Chunks) {
		chunkText = pctx.Chunks[chunkIdx].Text
	}
	return ContextBundle{
		SystemPrompt:  a.readPrompt("pass1_extraction"),
		UserPrompt:    chunkText,
		FileURIs:      []string{pctx.FileURI},
		TokenEstimate: estimateTokens(chunkText),
	}
}

func (a *DefaultAssembler) ForPass2Normal(pctx *pipeline.PipelineContext) ContextBundle {
	return ContextBundle{
		SystemPrompt:  a.readPrompt("pass2_synthesis"),
		UserPrompt:    strings.Join(pctx.Outline, "\n"),
		TokenEstimate: estimateTokens(strings.Join(pctx.Outline, "\n")) + len(pctx.SynthesizedNodes)*20,
	}
}

func (a *DefaultAssembler) ForPass2Lite(pctx *pipeline.PipelineContext, sectionIdx int) ContextBundle {
	return a.ForPass2Normal(pctx)
}

func (a *DefaultAssembler) ForPass2Final(pctx *pipeline.PipelineContext) ContextBundle {
	return a.ForPass2Normal(pctx)
}

func (a *DefaultAssembler) ForHTMLSummary(pctx *pipeline.PipelineContext, nodeLocalID string) ContextBundle {
	return ContextBundle{
		SystemPrompt:  a.readPrompt("html_summary"),
		UserPrompt:    fmt.Sprintf("node=%s", nodeLocalID),
		TokenEstimate: 128,
	}
}

func (a *DefaultAssembler) readPrompt(name string) string {
	path := filepath.Join(a.promptDir, name, "v1.txt")
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
