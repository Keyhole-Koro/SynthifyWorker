package pipeline

import "github.com/synthify/backend/packages/shared/jobstatus"

func (p *PipelineContext) JobStatusPayload() jobstatus.Payload {
	return jobstatus.Payload{
		JobID:       p.JobID,
		JobType:     p.JobType,
		DocumentID:  p.DocumentID,
		WorkspaceID: p.WorkspaceID,
		TreeID:      p.TreeID,
	}
}
