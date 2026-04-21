package workercontext

import "github.com/synthify/backend/worker/pkg/worker/pipeline"

type ContextBundle struct {
	SystemPrompt  string
	UserPrompt    string
	SourceFiles   []pipeline.SourceFile
	TokenEstimate int
	PromptName    string
	PromptVersion string
	SchemaVersion string
}

type Assembler interface {
	ForChunking(pctx *pipeline.PipelineContext) ContextBundle
	ForBriefGeneration(pctx *pipeline.PipelineContext) ContextBundle
	ForPass1(pctx *pipeline.PipelineContext, chunkIdx int) ContextBundle
	ForPass2Normal(pctx *pipeline.PipelineContext) ContextBundle
	ForPass2Lite(pctx *pipeline.PipelineContext, sectionIdx int) ContextBundle
	ForPass2Final(pctx *pipeline.PipelineContext) ContextBundle
	ForHTMLSummary(pctx *pipeline.PipelineContext, nodeLocalID string) ContextBundle
}
