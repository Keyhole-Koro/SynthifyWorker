package stages

import (
	"context"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type NormalizationStage struct{}

func (s *NormalizationStage) Name() pipeline.StageName { return pipeline.StageNormalization }

func (s *NormalizationStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	return nil
}
