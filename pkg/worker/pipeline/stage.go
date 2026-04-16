package pipeline

import "context"

type StageName string

const (
	StageRawIntake             StageName = "raw_intake"
	StageNormalization         StageName = "normalization"
	StageTextExtraction        StageName = "text_extraction"
	StageSemanticChunking      StageName = "semantic_chunking"
	StageBriefGeneration       StageName = "brief_generation"
	StagePass1Extraction       StageName = "pass1_extraction"
	StagePass2Synthesis        StageName = "pass2_synthesis"
	StagePersistence           StageName = "persistence"
	StageHTMLSummaryGeneration StageName = "html_summary_generation"
)

type Stage interface {
	Name() StageName
	Run(ctx context.Context, pctx *PipelineContext) error
}
