package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/worker/pkg/worker/agents"
	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	treev1connect "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
	"github.com/synthify/backend/packages/shared/job/lifecycle"
	"github.com/synthify/backend/packages/shared/job/log"
	"github.com/synthify/backend/packages/shared/job/status"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/storage"
	"github.com/synthify/backend/packages/shared/util"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/api/idtoken"
)

var (
	ErrApprovalRequired = domain.ErrApprovalRequired
	ErrPlanRejected     = domain.ErrPlanRejected
)

type Repository interface {
	repository.AccountRepository
	repository.WorkspaceRepository
	repository.DocumentRepository
	repository.TreeRepository
	repository.ItemRepository
	repository.CheckpointRepository
}

type Worker struct {
	orchestrator *agents.Orchestrator
	repo         Repository
	lifecycle    *joblifecycle.Service
	status       jobstatus.Notifier
	runner       *runner.Runner
	logger       applog.Logger
}

type ExecutePlanRequest = domain.ExecutePlanRequest

func NewWorker(repo Repository, m model.LLM, embedder base.Embedder, llmClient base.LLMClient, fs *storage.FileSystem, logger applog.Logger) (*Worker, error) {
	return NewWorkerWithNotifier(repo, repo, nil, m, embedder, llmClient, fs, logger)
}

func NewWorkerWithNotifier(repo Repository, treeRepo Repository, notifier jobstatus.Notifier, m model.LLM, embedder base.Embedder, llmClient base.LLMClient, fs *storage.FileSystem, logger applog.Logger) (*Worker, error) {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	usage := base.NewUsageLimiter(treeRepo, logger)
	b := &base.Context{
		Repo:     treeRepo,
		Embedder: embedder,
		LLM:      base.NewCountingLLMClient(llmClient, usage),
		Usage:    usage,
		FS:       fs,
		Logger:   logger,
	}
	orch, err := agents.NewOrchestrator(m, b, repo, fs)
	if err != nil {
		return nil, err
	}

	r, err := runner.New(runner.Config{
		AppName:           "synthify-worker",
		Agent:             orch.Agent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, err
	}

	return &Worker{
		orchestrator: orch,
		repo:         repo,
		lifecycle:    joblifecycle.New(repo, notifier, logger),
		status:       notifier,
		runner:       r,
		logger:       logger,
	}, nil
}

func (w *Worker) Process(ctx context.Context, req ExecutePlanRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       req.JobID,
		WorkspaceID: req.WorkspaceID,
		DocumentID:  req.DocumentID,
		Level:       joblog.INFO,
		Event:       "job.running",
		Message:     fmt.Sprintf("LLM worker processing job (doc: %s)", req.DocumentID),
		Detail:      map[string]any{"document_id": req.DocumentID},
	})

	job, err := w.repo.GetProcessingJob(ctx, req.JobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", req.JobID, err)
	}

	if _, err := w.repo.GetDocument(ctx, req.DocumentID); err != nil {
		return fmt.Errorf("get document %s: %w", req.DocumentID, err)
	}

	payload := jobstatus.Payload{
		JobID:       req.JobID,
		DocumentID:  req.DocumentID,
		WorkspaceID: req.WorkspaceID,
		TreeID:      req.TreeID,
		JobType:     job.JobType.String(),
	}

	if approvals, err := w.repo.ListJobApprovalRequests(ctx, req.JobID); err == nil {
		for _, a := range approvals {
			if a.Status == "rejected" {
				return ErrPlanRejected
			}
		}
	}

	if err := w.lifecycle.MarkRunning(ctx, payload); err != nil {
		return fmt.Errorf("mark job running: %w", err)
	}

	if err := w.orchestrator.ProcessDocument(ctx, w.runner, req.JobID, req.DocumentID, req.WorkspaceID, req.FileURI, req.Filename, req.MimeType); err != nil {
		w.failJob(ctx, req, payload, err)
		return err
	}

	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       req.JobID,
		WorkspaceID: req.WorkspaceID,
		DocumentID:  req.DocumentID,
		Level:       joblog.INFO,
		Event:       "job.completed",
		Message:     "LLM worker job completed successfully",
	})
	if err := w.lifecycle.Complete(ctx, payload); err != nil {
		return fmt.Errorf("complete job in repo: %w", err)
	}

	return nil
}

func (w *Worker) failJob(ctx context.Context, req ExecutePlanRequest, payload jobstatus.Payload, cause error) {
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       req.JobID,
		WorkspaceID: req.WorkspaceID,
		DocumentID:  req.DocumentID,
		Level:       joblog.ERROR,
		Event:       "job.failed",
		Message:     fmt.Sprintf("Agent execution failed: %v", cause),
		Detail:      map[string]any{"error": cause.Error()},
	})
	w.lifecycle.TryFail(ctx, payload, cause.Error())
}

type Planner struct {
	repo   Repository
	llm    model.LLM
	logger applog.Logger
}

func NewPlanner(repo Repository, llm model.LLM, logger applog.Logger) *Planner {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &Planner{repo: repo, llm: llm, logger: logger}
}

func (p *Planner) GenerateExecutionPlan(ctx context.Context, req ExecutePlanRequest) (*domain.JobExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := p.repo.GetDocument(ctx, req.DocumentID); err != nil {
		return nil, fmt.Errorf("get document %s: %w", req.DocumentID, err)
	}
	signals, err := p.repo.GetJobPlanningSignals(ctx, req.DocumentID, req.WorkspaceID, req.TreeID)
	if err != nil {
		p.logger.Error(ctx, "planner.get_signals_failed", err, map[string]any{"doc_id": req.DocumentID})
	}
	payload := map[string]any{
		"steps": []map[string]any{
			{"name": "text_extraction", "operation": treev1.JobOperation_JOB_OPERATION_READ_DOCUMENT.String(), "risk_tier": "tier_0"},
			{"name": "semantic_chunking", "operation": treev1.JobOperation_JOB_OPERATION_INVOKE_LLM.String(), "risk_tier": "tier_0"},
			{"name": "goal_driven_synthesis", "operation": treev1.JobOperation_JOB_OPERATION_CREATE_ITEM.String(), "risk_tier": "tier_1"},
			{"name": "persistence", "operation": treev1.JobOperation_JOB_OPERATION_CREATE_ITEM.String(), "risk_tier": "tier_1"},
			{"name": "evaluation", "operation": treev1.JobOperation_JOB_OPERATION_EMIT_EVAL.String(), "risk_tier": "tier_0"},
		},
		"document_id":  req.DocumentID,
		"workspace_id": req.WorkspaceID,
		"tree_id":      req.TreeID,
		"signals":      signals,
	}
	plan := &domain.JobExecutionPlan{
		PlanID:    "plan_" + req.JobID,
		JobID:     req.JobID,
		Status:    "approved",
		Summary:   "Extract text, chunk semantically, synthesize knowledge tree items, persist mutations, and evaluate the job.",
		PlanJSON:  util.MustJSON(payload),
		CreatedBy: "llm_worker",
	}
	if err := p.repo.UpsertJobExecutionPlan(ctx, req.JobID, plan); err != nil {
		return nil, fmt.Errorf("failed to upsert execution plan: %w", err)
	}
	return plan, nil
}

type JobEvaluator struct {
	agent  *agents.Evaluator
	repo   Repository
	logger applog.Logger
}

func NewJobEvaluator(repo Repository, llmClient llm.Client, logger applog.Logger) *JobEvaluator {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &JobEvaluator{repo: repo, agent: agents.NewEvaluator(llmClient), logger: logger}
}

func (e *JobEvaluator) Evaluate(ctx context.Context, jobID string) (*domain.JobEvaluationResult, error) {
	job, err := e.repo.GetProcessingJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", jobID, err)
	}

	rootID, err := e.repo.GetWorkspaceRootItemID(ctx, job.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("root item for workspace %s: %w", job.WorkspaceID, err)
	}

	subtree, err := e.repo.GetSubtree(ctx, rootID, 5)
	if err != nil {
		return nil, fmt.Errorf("get subtree: %w", err)
	}

	treeJSON, err := json.Marshal(subtree)
	if err != nil {
		return nil, fmt.Errorf("marshal tree: %w", err)
	}

	out, err := e.agent.EvaluateTree(ctx, string(treeJSON))
	if err != nil {
		return nil, err
	}

	findings := make([]string, len(out.Findings))
	copy(findings, out.Findings)

	result := &domain.JobEvaluationResult{
		JobID:    jobID,
		Passed:   out.Passed,
		Score:    int32(out.Score),
		Summary:  out.Summary,
		Findings: findings,
		Status:   "failed",
	}
	if out.Passed {
		result.Status = "passed"
	}

	e.repo.UpsertJobEvaluation(ctx, jobID, result)
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       jobID,
		WorkspaceID: job.WorkspaceID,
		DocumentID:  job.DocumentID,
		Level:       joblog.INFO,
		Event:       "evaluation.completed",
		Message:     fmt.Sprintf("evaluation: passed=%v score=%d findings=%d", result.Passed, result.Score, len(result.Findings)),
		Detail: map[string]any{
			"passed":   result.Passed,
			"score":    result.Score,
			"findings": result.Findings,
		},
	})
	return result, nil
}

type HTTPDispatcher struct {
	baseURL string
}

func NewHTTPDispatcher(baseURL string) *HTTPDispatcher {
	return &HTTPDispatcher{baseURL: baseURL}
}

func (d *HTTPDispatcher) GenerateExecutionPlan(ctx context.Context, req ExecutePlanRequest) error {
	httpClient, err := d.httpClient(ctx)
	if err != nil {
		return fmt.Errorf("http client: %w", err)
	}
	client := treev1connect.NewWorkerServiceClient(httpClient, strings.TrimRight(d.baseURL, "/"))
	rpcReq := connect.NewRequest(&treev1.GenerateExecutionPlanRequest{
		JobId:       req.JobID,
		JobType:     req.JobType,
		DocumentId:  req.DocumentID,
		WorkspaceId: req.WorkspaceID,
		TreeId:      req.TreeID,
		Filename:    req.Filename,
		MimeType:    req.MimeType,
	})
	if _, err = client.GenerateExecutionPlan(ctx, rpcReq); err != nil {
		return fmt.Errorf("GenerateExecutionPlan rpc: %w", err)
	}
	return nil
}

func (d *HTTPDispatcher) ExecuteApprovedPlan(ctx context.Context, req ExecutePlanRequest) error {
	httpClient, err := d.httpClient(ctx)
	if err != nil {
		return fmt.Errorf("http client: %w", err)
	}
	client := treev1connect.NewWorkerServiceClient(httpClient, strings.TrimRight(d.baseURL, "/"))
	rpcReq := connect.NewRequest(&treev1.ExecuteApprovedPlanRequest{
		JobId:       req.JobID,
		JobType:     req.JobType,
		DocumentId:  req.DocumentID,
		WorkspaceId: req.WorkspaceID,
		TreeId:      req.TreeID,
		FileUri:     req.FileURI,
		Filename:    req.Filename,
		MimeType:    req.MimeType,
	})
	_, err = client.ExecuteApprovedPlan(ctx, rpcReq)
	if err != nil && connect.CodeOf(err) == connect.CodeFailedPrecondition {
		return ErrApprovalRequired
	}
	return err
}

func (d *HTTPDispatcher) httpClient(ctx context.Context) (*http.Client, error) {
	baseURL := strings.TrimRight(d.baseURL, "/")
	if !strings.HasPrefix(baseURL, "https://") {
		return http.DefaultClient, nil
	}
	return idtoken.NewClient(ctx, baseURL)
}
