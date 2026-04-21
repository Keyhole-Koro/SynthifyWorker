package worker

import (
	"context"
	"errors"
	"net/http"

	connect "connectrpc.com/connect"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type ConnectHandler struct {
	processor interface {
		Process(ctx context.Context, pctx *pipeline.PipelineContext) error
	}
	token string
}

func NewConnectHandler(processor interface {
	Process(ctx context.Context, pctx *pipeline.PipelineContext) error
}, token string) *ConnectHandler {
	return &ConnectHandler{processor: processor, token: token}
}

func (h *ConnectHandler) ProcessPipeline(ctx context.Context, req *connect.Request[graphv1.ProcessPipelineRequest]) (*connect.Response[graphv1.ProcessPipelineResponse], error) {
	if h.token != "" && req.Header().Get("X-Worker-Token") != h.token {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	dispatchReq := DispatchRequest{
		JobID:       req.Msg.GetJobId(),
		JobType:     req.Msg.GetJobType(),
		DocumentID:  req.Msg.GetDocumentId(),
		WorkspaceID: req.Msg.GetWorkspaceId(),
		GraphID:     req.Msg.GetGraphId(),
		FileURI:     req.Msg.GetFileUri(),
		Filename:    req.Msg.GetFilename(),
		MimeType:    req.Msg.GetMimeType(),
	}
	if err := validateDispatchRequest(dispatchReq); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.processor.Process(ctx, &pipeline.PipelineContext{
		JobID:       dispatchReq.JobID,
		JobType:     dispatchReq.JobType,
		DocumentID:  dispatchReq.DocumentID,
		WorkspaceID: dispatchReq.WorkspaceID,
		GraphID:     dispatchReq.GraphID,
		FileURI:     dispatchReq.FileURI,
		Filename:    dispatchReq.Filename,
		MimeType:    dispatchReq.MimeType,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&graphv1.ProcessPipelineResponse{Status: "ok"}), nil
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
