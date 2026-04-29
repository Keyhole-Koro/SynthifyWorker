package tools

import (
	"fmt"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type PersistenceArgs struct {
	JobID       string                     `json:"job_id"`
	DocumentID  string                     `json:"document_id"`
	WorkspaceID string                     `json:"workspace_id"`
	Items       []pipeline.SynthesizedItem `json:"items"`
}

type PersistenceResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewPersistenceTool writes synthesized items into the backing repository.
// Input schema: PersistenceArgs{job_id: string, document_id: string, workspace_id: string, items: []pipeline.SynthesizedItem}.
// Output schema: PersistenceResult{success: bool, message: string}.
func NewPersistenceTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "persist_knowledge_tree",
		Description: "Permanently saves the synthesized knowledge tree items to the database.",
	}, func(ctx tool.Context, args PersistenceArgs) (PersistenceResult, error) {
		if len(args.Items) == 0 {
			return PersistenceResult{Success: false, Message: "No items to persist"}, nil
		}
		if base == nil || base.Repo == nil {
			return PersistenceResult{}, fmt.Errorf("repository is not configured")
		}
		capability, ok := base.Repo.GetJobCapability(ctx, args.JobID)
		if !ok || capability == nil {
			return PersistenceResult{}, fmt.Errorf("job capability not found: %s", args.JobID)
		}

		itemIDs := make(map[string]string, len(args.Items))
		rootID, _ := base.Repo.GetWorkspaceRootItemID(ctx, args.WorkspaceID)
		created := 0
		for _, item := range args.Items {
			parentID := rootID
			if mapped := itemIDs[item.ParentLocalID]; mapped != "" {
				parentID = mapped
			}
			label := strings.TrimSpace(item.Label)
			if label == "" {
				label = item.LocalID
			}
			createdItem := base.Repo.CreateStructuredItemWithCapability(
				ctx,
				capability,
				args.JobID,
				args.DocumentID,
				args.WorkspaceID,
				label,
				item.Level,
				item.Description,
				item.SummaryHTML,
				"llm_worker",
				parentID,
				item.SourceChunkIDs,
			)
			if createdItem == nil {
				return PersistenceResult{}, fmt.Errorf("failed to create item %q", label)
			}
			itemIDs[item.LocalID] = createdItem.ItemID
			for _, chunkID := range item.SourceChunkIDs {
				if err := base.Repo.UpsertItemSource(ctx, createdItem.ItemID, args.DocumentID, chunkID, item.Description, 0.75); err != nil {
					return PersistenceResult{}, err
				}
			}
			created++
		}

		return PersistenceResult{Success: true, Message: fmt.Sprintf("Successfully persisted %d items", created)}, nil
	})
}
