package worker

import (
	"context"
	"path/filepath"

	"github.com/Keyhole-Koro/SynthifyShared/config"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"github.com/synthify/backend/worker/pkg/worker/stages"
)

type documentRepo interface {
	MarkProcessingJobRunning(jobID string) bool
	UpdateProcessingJobStage(jobID, stage string) bool
	FailProcessingJob(jobID, errorMessage string) bool
	CompleteProcessingJob(jobID string) bool
	SaveDocumentChunks(documentID string, chunks []*domain.DocumentChunk) error
}

type graphRepo interface {
	GetWorkspaceRootNodeID(graphID string) (string, bool)
	CreateStructuredNode(graphID, label, category string, level int, entityType, description, summaryHTML, createdBy string) *domain.Node
	CreateEdge(graphID, sourceNodeID, targetNodeID, edgeType, description string) *domain.Edge
	UpsertNodeSource(nodeID, documentID, chunkID, sourceText string, confidence float64) error
	UpsertEdgeSource(edgeID, documentID, chunkID, sourceText string, confidence float64) error
	UpdateNodeSummaryHTML(nodeID, summaryHTML string) bool
}

type Processor struct {
	runner *pipeline.PipelineRunner
}

type combinedRepo struct {
	documentRepo
	graphRepo
}

func NewProcessor(jobRepo documentRepo, graphRepo graphRepo) *Processor {
	notifier := jobstatus.NewNotifier(context.Background(), "")
	return NewProcessorWithNotifier(jobRepo, graphRepo, notifier)
}

func NewProcessorWithNotifier(jobRepo documentRepo, graphRepo graphRepo, notifier jobstatus.Notifier) *Processor {
	assembler := workercontext.NewDefaultAssembler(filepath.Join("worker", "pkg", "worker", "prompts"))
	llmConfig := config.LoadLLM()
	var llmClient workerllm.Client
	if llmConfig.Enabled() {
		llmClient = workerllm.NewRetryingClient(workerllm.NewGeminiClient(llmConfig), 2)
	}
	runner := pipeline.NewRunner(
		jobRepo,
		notifier,
		&stages.RawIntakeStage{},
		&stages.NormalizationStage{},
		&stages.TextExtractionStage{},
		stages.NewSemanticChunkingStage(assembler, llmClient),
		stages.NewBriefGenerationStage(assembler, llmClient),
		stages.NewPass1ExtractionStage(assembler, llmClient, 5),
		stages.NewPass2SynthesisStage(assembler, llmClient),
		stages.NewPersistenceStage(combinedRepo{documentRepo: jobRepo, graphRepo: graphRepo}),
		stages.NewHTMLSummaryGenerationStage(graphRepo, assembler, llmClient, 10),
	)
	return &Processor{runner: runner}
}

func (p *Processor) Process(ctx context.Context, pctx *pipeline.PipelineContext) error {
	return p.runner.Run(ctx, pctx)
}
