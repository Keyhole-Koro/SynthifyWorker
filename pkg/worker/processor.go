package worker

import (
	"context"
	"errors"
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
	GetJobCapability(jobID string) (*domain.JobCapability, bool)
	MarkProcessingJobRunning(jobID string) bool
	UpdateProcessingJobStage(jobID, stage string) bool
	FailProcessingJob(jobID, errorMessage string) bool
	CompleteProcessingJob(jobID string) bool
	SaveDocumentChunks(documentID string, chunks []*domain.DocumentChunk) error
}

type graphRepo interface {
	GetWorkspaceRootNodeID(graphID string) (string, bool)
	CreateStructuredNodeWithCapability(capability *domain.JobCapability, jobID, documentID, graphID, label string, level int, description, summaryHTML, createdBy string, sourceChunkIDs []string) *domain.Node
	CreateEdgeWithCapability(capability *domain.JobCapability, jobID, documentID, graphID, sourceNodeID, targetNodeID, edgeType, description string, sourceChunkIDs []string) *domain.Edge
	UpsertNodeSource(nodeID, documentID, chunkID, sourceText string, confidence float64) error
	UpsertEdgeSource(edgeID, documentID, chunkID, sourceText string, confidence float64) error
	UpdateNodeSummaryHTMLWithCapability(capability *domain.JobCapability, jobID, nodeID, summaryHTML string) bool
}

type Processor struct {
	runner  *pipeline.PipelineRunner
	jobRepo documentRepo
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
		stages.NewGoalDrivenSynthesisStage(llmClient),
		stages.NewPersistenceStage(combinedRepo{documentRepo: jobRepo, graphRepo: graphRepo}),
		stages.NewHTMLSummaryGenerationStage(graphRepo, assembler, llmClient, 10),
	)
	return &Processor{runner: runner, jobRepo: jobRepo}
}

func (p *Processor) Process(ctx context.Context, pctx *pipeline.PipelineContext) error {
	if pctx.Capability == nil {
		if pctx.JobID == "" {
			return errors.New("job capability is required")
		}
		capability, ok := p.jobRepo.GetJobCapability(pctx.JobID)
		if !ok || capability == nil {
			return errors.New("job capability not found")
		}
		pctx.Capability = capability
	}
	return p.runner.Run(ctx, pctx)
}
