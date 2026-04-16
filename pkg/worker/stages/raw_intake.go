package stages

import (
	"context"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type RawIntakeStage struct{}

func (s *RawIntakeStage) Name() pipeline.StageName { return pipeline.StageRawIntake }

func (s *RawIntakeStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	pctx.SourceFiles = []pipeline.SourceFile{{
		Filename: pctx.Filename,
		URI:      pctx.FileURI,
		MimeType: pctx.MimeType,
	}}
	return nil
}
