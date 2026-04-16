package eval

import (
	"context"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type PromptEvolver interface {
	Evolve(ctx context.Context, req EvolveRequest) (EvolveResult, error)
}

type EvolveRequest struct {
	Stage         pipeline.StageName
	CurrentPrompt string
	Failures      []GroundingFailure
	MaxIterations int
}

type EvolveResult struct {
	ImprovedPrompt string
	Reasoning      string
}
