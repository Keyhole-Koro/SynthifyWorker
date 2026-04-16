package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type DispatchRequest struct {
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
	Dispatch(ctx context.Context, req DispatchRequest) error
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

func (d *InlineDispatcher) Dispatch(ctx context.Context, req DispatchRequest) error {
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
	client  *http.Client
}

func NewHTTPDispatcher(baseURL, token string) *HTTPDispatcher {
	return &HTTPDispatcher{
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

func (d *HTTPDispatcher) Dispatch(ctx context.Context, req DispatchRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/internal/pipeline", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if d.token != "" {
		httpReq.Header.Set("X-Worker-Token", d.token)
	}
	res, err := d.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("worker dispatch failed: %s", res.Status)
	}
	return nil
}
