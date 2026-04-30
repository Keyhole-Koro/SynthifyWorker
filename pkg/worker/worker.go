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
	sharedpipeline "github.com/Keyhole-Koro/SynthifyShared/pipeline"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/Keyhole-Koro/SynthifyShared/util"
	"github.com/synthify/backend/worker/pkg/worker/agents"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"github.com/synthify/backend/worker/pkg/worker/sourcefiles"
	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/api/idtoken"
	"google.golang.org/genai"
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
	embedder     base.Embedder
}

type ExecutePlanRequest = domain.ExecutePlanRequest

func NewWorker(repo Repository, m model.LLM) (*Worker, error) {
	return NewWorkerWithNotifier(repo, repo, nil, m, nil)
}

func NewWorkerWithNotifier(repo Repository, treeRepo Repository, notifier jobstatus.Notifier, m model.LLM, embedder base.Embedder) (*Worker, error) {
	b := &base.Context{
		Repo:     treeRepo,
		Embedder: embedder,
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
		embedder:     embedder,
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

	if err := w.processDocument(ctx, req, payload); err != nil {
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

func (w *Worker) processDocument(ctx context.Context, req ExecutePlanRequest, payload jobstatus.Payload) error {
	if w.status != nil {
		w.status.StageProgress(ctx, payload, string(pipeline.StageTextExtraction), 10, "Reading source document")
	}
	w.repo.UpdateProcessingJobStage(ctx, req.JobID, string(pipeline.StageTextExtraction))
	rawText, err := w.loadRawText(ctx, req)
	if err != nil {
		return err
	}

	if w.status != nil {
		w.status.StageProgress(ctx, payload, string(pipeline.StageSemanticChunking), 35, "Chunking document")
	}
	w.repo.UpdateProcessingJobStage(ctx, req.JobID, string(pipeline.StageSemanticChunking))
	chunks := sharedpipeline.BuildChunks(req.DocumentID, rawText)
	if len(chunks) == 0 {
		return fmt.Errorf("document produced no chunks")
	}
	if w.embedder == nil {
		return fmt.Errorf("embedder is required: configure GEMINI_API_KEY")
	}
	for _, chunk := range chunks {
		vec, err := w.embedder.EmbedText(ctx, chunk.Heading+" "+chunk.Text)
		if err != nil {
			return fmt.Errorf("embed chunk %s: %w", chunk.ChunkID, err)
		}
		chunk.Embedding = vec.Slice()
	}
	if err := w.repo.SaveDocumentChunks(ctx, req.DocumentID, chunks); err != nil {
		return err
	}

	if w.status != nil {
		w.status.StageProgress(ctx, payload, string(pipeline.StageGoalDrivenSynthesis), 65, "Synthesizing knowledge items")
	}
	w.repo.UpdateProcessingJobStage(ctx, req.JobID, string(pipeline.StageGoalDrivenSynthesis))
	items := synthesizeItems(req.DocumentID, chunks)
	if len(items) == 0 {
		return fmt.Errorf("no items synthesized")
	}

	_ = w.runAgentBestEffort(ctx, req, rawText)

	if w.status != nil {
		w.status.StageProgress(ctx, payload, string(pipeline.StagePersistence), 90, "Saving generated knowledge")
	}
	w.repo.UpdateProcessingJobStage(ctx, req.JobID, string(pipeline.StagePersistence))
	return w.persistItems(ctx, req, items, chunks)
}

func (w *Worker) loadRawText(ctx context.Context, req ExecutePlanRequest) (string, error) {
	if req.FileURI != "" {
		source := domain.SourceFile{Filename: req.Filename, URI: req.FileURI, MimeType: req.MimeType}
		if err := sourcefiles.Fetch(ctx, &source); err != nil {
			return "", err
		}
		text := strings.TrimSpace(strings.ReplaceAll(string(source.Content), "\x00", ""))
		if text != "" {
			return text, nil
		}
	}
	if chunks, ok := w.repo.GetDocumentChunks(ctx, req.DocumentID); ok && len(chunks) > 0 {
		var b strings.Builder
		for _, chunk := range chunks {
			if chunk.Heading != "" {
				b.WriteString(chunk.Heading)
				b.WriteByte('\n')
			}
			b.WriteString(chunk.Text)
			b.WriteString("\n\n")
		}
		return strings.TrimSpace(b.String()), nil
	}
	return "", fmt.Errorf("no source text available for document %s", req.DocumentID)
}

func (w *Worker) runAgentBestEffort(ctx context.Context, req ExecutePlanRequest, rawText string) error {
	if w.runner == nil {
		return nil
	}
	message := fmt.Sprintf("Review job_id=%s document_id=%s workspace_id=%s for processing context only. Do not call tools or persist data; the deterministic worker pipeline will perform mutations. Source text:\n%s", req.JobID, req.DocumentID, req.WorkspaceID, util.TruncateRunes(rawText, 12000))
	sessionID := req.JobID
	for _, err := range w.runner.Run(ctx, "worker", sessionID, genai.NewContentFromText(message, genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			log.Printf("ADK agent run skipped/failed for job %s: %v", req.JobID, err)
			return nil
		}
	}
	return nil
}

func (w *Worker) persistItems(ctx context.Context, req ExecutePlanRequest, items []domain.SynthesizedItem, chunks []*domain.DocumentChunk) error {
	capability, ok := w.repo.GetJobCapability(ctx, req.JobID)
	if !ok || capability == nil {
		return fmt.Errorf("job capability not found: %s", req.JobID)
	}
	rootID, _ := w.repo.GetWorkspaceRootItemID(ctx, req.WorkspaceID)
	chunkByID := make(map[string]*domain.DocumentChunk, len(chunks))
	for _, chunk := range chunks {
		chunkByID[chunk.ChunkID] = chunk
	}
	itemIDs := make(map[string]string, len(items))
	for _, item := range items {
		parentID := rootID
		if item.ParentLocalID != "" {
			parentID = util.FirstNonEmpty(itemIDs[item.ParentLocalID], rootID)
		}
		created := w.repo.CreateStructuredItemWithCapability(
			ctx,
			capability,
			req.JobID,
			req.DocumentID,
			req.WorkspaceID,
			item.Label,
			item.Level,
			item.Description,
			item.SummaryHTML,
			"llm_worker",
			parentID,
			item.SourceChunkIDs,
		)
		if created == nil {
			return fmt.Errorf("failed to create item %q", item.Label)
		}
		itemIDs[item.LocalID] = created.ItemID
		for _, chunkID := range item.SourceChunkIDs {
			sourceText := item.Description
			if chunk := chunkByID[chunkID]; chunk != nil {
				sourceText = util.TruncateRunes(chunk.Text, 1000)
			}
			if err := w.repo.UpsertItemSource(ctx, created.ItemID, req.DocumentID, chunkID, sourceText, 0.75); err != nil {
				return err
			}
		}
		_ = w.repo.LogToolCall(ctx, req.JobID, "persist_knowledge_tree", util.MustJSON(map[string]any{"item": item.Label}), util.MustJSON(created), 0)
	}
	return nil
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

func synthesizeItems(documentID string, chunks []*domain.DocumentChunk) []domain.SynthesizedItem {
	items := make([]domain.SynthesizedItem, 0, len(chunks))
	for i, chunk := range chunks {
		label := strings.TrimSpace(chunk.Heading)
		if label == "" {
			label = fmt.Sprintf("Section %d", i+1)
		}
		description := util.TruncateRunes(strings.Join(strings.Fields(chunk.Text), " "), 360)
		items = append(items, domain.SynthesizedItem{
			LocalID:        fmt.Sprintf("chunk_%d", i),
			Label:          label,
			Level:          1,
			Description:    description,
			SummaryHTML:    "<p>" + util.HTMLEscape(description) + "</p>",
			SourceChunkIDs: []string{fmt.Sprintf("%s_chunk_%d", documentID, i)},
		})
	}
	return items
}
