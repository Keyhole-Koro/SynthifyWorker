package eval

import (
	"context"
	"encoding/json"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type GroundingJudge interface {
	Judge(ctx context.Context, req JudgeRequest) (GroundingScore, error)
}

type JudgeRequest struct {
	Stage         pipeline.StageName
	SourceText    string
	ContextBundle workercontext.ContextBundle
	Output        json.RawMessage
}

type GroundingScore struct {
	Stage     pipeline.StageName
	Overall   float64
	Failures  []GroundingFailure
	Reasoning string
}

type GroundingFailure struct {
	NodeLocalID string
	Label       string
	Issue       string
	Detail      string
}
