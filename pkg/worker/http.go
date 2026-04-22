package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/config"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type InternalHandler struct {
	processor interface {
		Process(ctx context.Context, pctx *pipeline.PipelineContext) error
	}
	token string
}

func NewInternalHandler(processor interface {
	Process(ctx context.Context, pctx *pipeline.PipelineContext) error
}, token string) *InternalHandler {
	return &InternalHandler{processor: processor, token: token}
}

func (h *InternalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.token != "" && r.Header.Get("X-Worker-Token") != h.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req ExecutePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateExecutePlanRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.processor.Process(r.Context(), &pipeline.PipelineContext{
		JobID:       req.JobID,
		JobType:     req.JobType,
		DocumentID:  req.DocumentID,
		WorkspaceID: req.WorkspaceID,
		GraphID:     req.GraphID,
		FileURI:     req.FileURI,
		Filename:    req.Filename,
		MimeType:    req.MimeType,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func validateExecutePlanRequest(req ExecutePlanRequest) error {
	switch {
	case req.JobID == "":
		return errors.New("job_id is required")
	case req.JobType == "":
		return errors.New("job_type is required")
	case req.DocumentID == "":
		return errors.New("document_id is required")
	case req.WorkspaceID == "":
		return errors.New("workspace_id is required")
	case req.GraphID == "":
		return errors.New("graph_id is required")
	case req.FileURI == "":
		return errors.New("file_uri is required")
	default:
		return nil
	}
}

func ServiceMode() string {
	return config.LoadService().Mode
}
