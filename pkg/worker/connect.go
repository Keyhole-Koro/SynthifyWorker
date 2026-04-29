package worker

import (
	"context"
	"errors"
	"net/http"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
)

type ConnectHandler struct {
	processor interface {
		Process(ctx context.Context, req ExecutePlanRequest) error
	}
	jobRepo interface {
		GetProcessingJob(ctx context.Context, jobID string) (*domain.DocumentProcessingJob, bool)
		GetJobCapability(ctx context.Context, jobID string) (*domain.JobCapability, bool)
		GetDocument(ctx context.Context, id string) (*domain.Document, bool)
		GetDocumentChunks(ctx context.Context, documentID string) ([]*domain.DocumentChunk, bool)
		GetJobPlanningSignals(ctx context.Context, documentID, workspaceID, treeID string) (*domain.JobPlanningSignals, bool)
		GetTreeByWorkspace(ctx context.Context, wsID string) ([]*domain.Item, bool)
		GetJobExecutionPlan(ctx context.Context, jobID string) (*domain.JobExecutionPlan, bool)
		UpsertJobExecutionPlan(ctx context.Context, jobID string, plan *domain.JobExecutionPlan) bool
		UpsertJobEvaluation(ctx context.Context, jobID string, result *domain.JobEvaluationResult) bool
		EvaluateJob(ctx context.Context, jobID string) (*domain.JobEvaluationResult, bool)
	}
	planner   *Planner
	evaluator *JobEvaluator
	token     string
}

func NewConnectHandler(processor interface {
	Process(ctx context.Context, req ExecutePlanRequest) error
}, jobRepo interface {
	GetProcessingJob(ctx context.Context, jobID string) (*domain.DocumentProcessingJob, bool)
	GetJobCapability(ctx context.Context, jobID string) (*domain.JobCapability, bool)
	GetDocument(ctx context.Context, id string) (*domain.Document, bool)
	GetDocumentChunks(ctx context.Context, documentID string) ([]*domain.DocumentChunk, bool)
	GetJobPlanningSignals(ctx context.Context, documentID, workspaceID, treeID string) (*domain.JobPlanningSignals, bool)
	GetTreeByWorkspace(ctx context.Context, wsID string) ([]*domain.Item, bool)
	GetJobExecutionPlan(ctx context.Context, jobID string) (*domain.JobExecutionPlan, bool)
	UpsertJobExecutionPlan(ctx context.Context, jobID string, plan *domain.JobExecutionPlan) bool
	UpsertJobEvaluation(ctx context.Context, jobID string, result *domain.JobEvaluationResult) bool
	EvaluateJob(ctx context.Context, jobID string) (*domain.JobEvaluationResult, bool)
}, planner *Planner, evaluator *JobEvaluator, token string) *ConnectHandler {
	return &ConnectHandler{
		processor: processor,
		jobRepo:   jobRepo,
		planner:   planner,
		evaluator: evaluator,
		token:     token,
	}
}

func (h *ConnectHandler) GenerateExecutionPlan(ctx context.Context, req *connect.Request[treev1.GenerateExecutionPlanRequest]) (*connect.Response[treev1.GenerateExecutionPlanResponse], error) {
	if h.token != "" && req.Header().Get("X-Worker-Token") != h.token {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	if req.Msg.GetJobId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}
	plan, err := h.planner.GenerateExecutionPlan(ctx, ExecutePlanRequest{
		JobID:       req.Msg.GetJobId(),
		JobType:     req.Msg.GetJobType(),
		DocumentID:  req.Msg.GetDocumentId(),
		WorkspaceID: req.Msg.GetWorkspaceId(),
		TreeID:      req.Msg.GetTreeId(),
		Filename:    req.Msg.GetFilename(),
		MimeType:    req.Msg.GetMimeType(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.GenerateExecutionPlanResponse{
		PlanId:   plan.PlanID,
		JobId:    plan.JobID,
		Status:   plan.Status,
		Summary:  plan.Summary,
		PlanJson: plan.PlanJSON,
	}), nil
}

func (h *ConnectHandler) ExecuteApprovedPlan(ctx context.Context, req *connect.Request[treev1.ExecuteApprovedPlanRequest]) (*connect.Response[treev1.ExecuteApprovedPlanResponse], error) {
	if h.token != "" && req.Header().Get("X-Worker-Token") != h.token {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	dispatchReq := ExecutePlanRequest{
		JobID:       req.Msg.GetJobId(),
		JobType:     req.Msg.GetJobType(),
		DocumentID:  req.Msg.GetDocumentId(),
		WorkspaceID: req.Msg.GetWorkspaceId(),
		TreeID:      req.Msg.GetTreeId(),
		FileURI:     req.Msg.GetFileUri(),
		Filename:    req.Msg.GetFilename(),
		MimeType:    req.Msg.GetMimeType(),
	}
	if err := validateExecutePlanRequest(dispatchReq); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.processor.Process(ctx, dispatchReq); err != nil {
		if errors.Is(err, ErrApprovalRequired) || errors.Is(err, ErrPlanRejected) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&treev1.ExecuteApprovedPlanResponse{Status: "ok"}), nil
}

func (h *ConnectHandler) EvaluateJobArtifact(ctx context.Context, req *connect.Request[treev1.EvaluateJobArtifactRequest]) (*connect.Response[treev1.EvaluateJobArtifactResponse], error) {
	if h.token != "" && req.Header().Get("X-Worker-Token") != h.token {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	if req.Msg.GetJobId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}

	var result *domain.JobEvaluationResult

	if h.evaluator != nil {
		var err error
		result, err = h.evaluator.Evaluate(ctx, req.Msg.GetJobId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		var ok bool
		result, ok = h.jobRepo.EvaluateJob(ctx, req.Msg.GetJobId())
		if !ok || result == nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("evaluation result not found"))
		}
	}

	return connect.NewResponse(&treev1.EvaluateJobArtifactResponse{
		Passed:        result.Passed,
		Status:        result.Status,
		Summary:       result.Summary,
		Score:         result.Score,
		Findings:      append([]string(nil), result.Findings...),
		MutationCount: result.MutationCount,
	}), nil
}

func RequireWorkerToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Worker-Token") != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
