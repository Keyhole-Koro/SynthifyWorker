package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	treev1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1/treev1connect"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/Keyhole-Koro/SynthifyShared/util"
	"github.com/synthify/backend/worker/pkg/worker/agents"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
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
}

type Worker struct {
	orchestrator *agents.Orchestrator
	repo         Repository
	status       jobstatus.Notifier
	runner       *runner.Runner
}

type ExecutePlanRequest = domain.ExecutePlanRequest

func NewWorker(repo Repository, m model.LLM, embedder base.Embedder, llmClient base.LLMClient) (*Worker, error) {
	return NewWorkerWithNotifier(repo, repo, nil, m, embedder, llmClient)
}

func NewWorkerWithNotifier(repo Repository, treeRepo Repository, notifier jobstatus.Notifier, m model.LLM, embedder base.Embedder, llmClient base.LLMClient) (*Worker, error) {
	b := &base.Context{
		Repo:     treeRepo,
		Embedder: embedder,
		LLM:      llmClient,
	}
	orch, err := agents.NewOrchestrator(m, b, repo)
	if err != nil {
		return nil, err
	}

	r, err := runner.New(runner.Config{
		AppName:           "synthify-worker",
		Agent:             orch.Agent(),
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, err
	}

	return &Worker{
		orchestrator: orch,
		repo:         repo,
		status:       notifier,
		runner:       r,
	}, nil
}

func (w *Worker) Process(ctx context.Context, req ExecutePlanRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	log.Printf("LLM worker processing job %s (doc: %s)", req.JobID, req.DocumentID)

	job, ok := w.repo.GetProcessingJob(ctx, req.JobID)
	if !ok {
		return fmt.Errorf("job %s not found", req.JobID)
	}

	if _, ok := w.repo.GetDocument(ctx, req.DocumentID); !ok {
		return fmt.Errorf("document %s not found", req.DocumentID)
	}

	payload := jobstatus.Payload{
		JobID:       req.JobID,
		DocumentID:  req.DocumentID,
		WorkspaceID: req.WorkspaceID,
		TreeID:      req.TreeID,
		JobType:     job.JobType.String(),
	}

	if approvals, ok := w.repo.ListJobApprovalRequests(ctx, req.JobID); ok {
		for _, a := range approvals {
			if a.Status == "rejected" {
				return ErrPlanRejected
			}
		}
	}

	w.repo.MarkProcessingJobRunning(ctx, req.JobID)
	if w.status != nil {
		w.status.Running(ctx, payload)
	}

	if err := w.processDocument(ctx, req); err != nil {
		log.Printf("Agent execution failed: %v", err)
		w.repo.FailProcessingJob(ctx, req.JobID, err.Error())
		if w.status != nil {
			w.status.Failed(ctx, payload, err.Error())
		}
		return err
	}

	log.Printf("LLM worker job %s completed successfully", req.JobID)
	w.repo.CompleteProcessingJob(ctx, req.JobID)
	if w.status != nil {
		w.status.Completed(ctx, payload)
	}

	return nil
}

func (w *Worker) processDocument(ctx context.Context, req ExecutePlanRequest) error {
	return w.orchestrator.ProcessDocument(ctx, w.runner, req.JobID, req.DocumentID, req.WorkspaceID, req.FileURI, req.Filename, req.MimeType)
}

type Planner struct {
	repo Repository
	llm  model.LLM
}

func NewPlanner(repo Repository, llm model.LLM) *Planner {
	return &Planner{repo: repo, llm: llm}
}

func (p *Planner) GenerateExecutionPlan(ctx context.Context, req ExecutePlanRequest) (*domain.JobExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, ok := p.repo.GetDocument(ctx, req.DocumentID); !ok {
		return nil, fmt.Errorf("document %s not found", req.DocumentID)
	}
	signals, _ := p.repo.GetJobPlanningSignals(ctx, req.DocumentID, req.WorkspaceID, req.TreeID)
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
	if !p.repo.UpsertJobExecutionPlan(ctx, req.JobID, plan) {
		return nil, fmt.Errorf("failed to upsert execution plan")
	}
	return plan, nil
}

type JobEvaluator struct {
	agent *agents.Evaluator
	repo  Repository
}

func NewJobEvaluator(repo Repository, llmClient llm.Client) *JobEvaluator {
	return &JobEvaluator{repo: repo, agent: agents.NewEvaluator(llmClient)}
}

func (e *JobEvaluator) Evaluate(ctx context.Context, jobID string) (*domain.JobEvaluationResult, error) {
	job, ok := e.repo.GetProcessingJob(ctx, jobID)
	if !ok {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	rootID, ok := e.repo.GetWorkspaceRootItemID(ctx, job.WorkspaceID)
	if !ok {
		return nil, fmt.Errorf("root item not found for workspace %s", job.WorkspaceID)
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
		return err
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
	_, err = client.GenerateExecutionPlan(ctx, rpcReq)
	return err
}

func (d *HTTPDispatcher) ExecuteApprovedPlan(ctx context.Context, req ExecutePlanRequest) error {
	httpClient, err := d.httpClient(ctx)
	if err != nil {
		return err
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

