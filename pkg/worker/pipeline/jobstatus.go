package pipeline

import "github.com/synthify/backend/internal/jobstatus"

func (p *PipelineContext) JobStatusPayload() jobstatus.Payload {
	return jobstatus.Payload{
		JobID:       p.JobID,
		JobType:     p.JobType,
		DocumentID:  p.DocumentID,
		WorkspaceID: p.WorkspaceID,
		GraphID:     p.GraphID,
	}
}
