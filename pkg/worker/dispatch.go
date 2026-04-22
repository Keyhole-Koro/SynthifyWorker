package worker

import (
	"context"
	"fmt"
	"net/http"

	connect "connectrpc.com/connect"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	graphv1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1/graphv1connect"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type ExecutePlanRequest struct {
	JobID       string `json:"job_id"`
	JobType     string `json:"job_type"`
	DocumentID  string `json:"document_id"`
	WorkspaceID string `json:"workspace_id"`
	GraphID     string `json:"graph_id"`
	FileURI     string `json:"file_uri"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
}

type Dispatcher interface {
	ExecuteApprovedPlan(ctx context.Context, req ExecutePlanRequest) error
}

type InlineDispatcher struct {
	processor interface {
		Process(ctx context.Context, pctx *pipeline.PipelineContext) error
	}
}

func NewInlineDispatcher(processor interface {
	Process(ctx context.Context, pctx *pipeline.PipelineContext) error
}) *InlineDispatcher {
	return &InlineDispatcher{processor: processor}
}

func (d *InlineDispatcher) ExecuteApprovedPlan(ctx context.Context, req ExecutePlanRequest) error {
	return d.processor.Process(ctx, &pipeline.PipelineContext{
		JobID:       req.JobID,
		JobType:     req.JobType,
		DocumentID:  req.DocumentID,
		WorkspaceID: req.WorkspaceID,
		GraphID:     req.GraphID,
		FileURI:     req.FileURI,
		Filename:    req.Filename,
		MimeType:    req.MimeType,
	})
}

type HTTPDispatcher struct {
	baseURL string
	token   string
	client  graphv1connect.WorkerServiceClient
}

func NewHTTPDispatcher(baseURL, token string) *HTTPDispatcher {
	return &HTTPDispatcher{
		baseURL: baseURL,
		token:   token,
		client:  graphv1connect.NewWorkerServiceClient(http.DefaultClient, baseURL),
	}
}

func (d *HTTPDispatcher) ExecuteApprovedPlan(ctx context.Context, req ExecutePlanRequest) error {
	connectReq := connect.NewRequest(&graphv1.ExecuteApprovedPlanRequest{
		JobId:       req.JobID,
		JobType:     req.JobType,
		DocumentId:  req.DocumentID,
		WorkspaceId: req.WorkspaceID,
		GraphId:     req.GraphID,
		FileUri:     req.FileURI,
		Filename:    req.Filename,
		MimeType:    req.MimeType,
	})
	if d.token != "" {
		connectReq.Header().Set("X-Worker-Token", d.token)
	}
	res, err := d.client.ExecuteApprovedPlan(ctx, connectReq)
	if err != nil {
		return err
	}
	if res.Msg.GetStatus() != "ok" {
		return fmt.Errorf("worker dispatch failed: status=%s", res.Msg.GetStatus())
	}
	return nil
}
