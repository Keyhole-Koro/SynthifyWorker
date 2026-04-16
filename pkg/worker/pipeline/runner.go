package pipeline

import (
	"context"
	"fmt"

	"github.com/synthify/backend/internal/jobstatus"
)

type JobRepository interface {
	MarkProcessingJobRunning(jobID string) bool
	UpdateProcessingJobStage(jobID, stage string) bool
	FailProcessingJob(jobID, errorMessage string) bool
	CompleteProcessingJob(jobID string) bool
}

type PipelineRunner struct {
	stages   []Stage
	jobRepo  JobRepository
	notifier jobstatus.Notifier
}

func NewRunner(jobRepo JobRepository, notifier jobstatus.Notifier, stages ...Stage) *PipelineRunner {
	return &PipelineRunner{stages: stages, jobRepo: jobRepo, notifier: notifier}
}

func (r *PipelineRunner) Run(ctx context.Context, pctx *PipelineContext) error {
	r.jobRepo.MarkProcessingJobRunning(pctx.JobID)
	if r.notifier != nil {
		r.notifier.Running(ctx, pctx.JobStatusPayload())
	}
	for _, stage := range r.stages {
		r.jobRepo.UpdateProcessingJobStage(pctx.JobID, string(stage.Name()))
		if r.notifier != nil {
			r.notifier.Stage(ctx, pctx.JobStatusPayload(), string(stage.Name()))
		}
		if err := stage.Run(ctx, pctx); err != nil {
			wrapped := fmt.Errorf("stage %s: %w", stage.Name(), err)
			r.jobRepo.FailProcessingJob(pctx.JobID, wrapped.Error())
			if r.notifier != nil {
				r.notifier.Failed(ctx, pctx.JobStatusPayload(), wrapped.Error())
			}
			return wrapped
		}
	}
	r.jobRepo.CompleteProcessingJob(pctx.JobID)
	if r.notifier != nil {
		r.notifier.Completed(ctx, pctx.JobStatusPayload())
	}
	return nil
}
