package io

import (
	"fmt"
	"strings"

	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type PersistenceArgs struct {
	JobID       string                   `json:"job_id"`
	DocumentID  string                   `json:"document_id"`
	WorkspaceID string                   `json:"workspace_id"`
	Items       []domain.SynthesizedItem `json:"items"`
}

type PersistenceResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NewPersistenceTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "persist_knowledge_tree",
		Description: "Permanently saves the synthesized knowledge tree items to the database.",
	}, func(ctx tool.Context, args PersistenceArgs) (PersistenceResult, error) {
		if len(args.Items) == 0 {
			return PersistenceResult{Success: false, Message: "No items to persist"}, nil
		}
		if b == nil || b.Repo == nil {
			return PersistenceResult{}, fmt.Errorf("repository is not configured")
		}
		if err := b.IncrementItemCreations(ctx, len(args.Items)); err != nil {
			return PersistenceResult{}, err
		}
		capability, err := b.Repo.GetJobCapability(ctx, args.JobID)
		if err != nil {
			return PersistenceResult{}, fmt.Errorf("job capability not found: %s (%w)", args.JobID, err)
		}

		itemIDs := make(map[string]string, len(args.Items))
		rootID, err := b.Repo.GetWorkspaceRootItemID(ctx, args.WorkspaceID)
		if err != nil {
			// Fallback or handle missing root
			rootID = ""
		}
		created := 0
		for _, item := range args.Items {
			parentID := rootID
			if mapped := itemIDs[item.ParentLocalID]; mapped != "" {
				parentID = mapped
			}
			title := strings.TrimSpace(item.Title)
			if title == "" {
				title = item.LocalID
			}
			createdItem := b.Repo.CreateStructuredItemWithCapability(
				ctx,
				capability,
				args.JobID,
				args.DocumentID,
				args.WorkspaceID,
				title,
				item.Level,
				item.Description,
				item.Content,
				item.OverrideCSS,
				"llm_worker",
				parentID,
				item.SourceChunkIDs,
			)
			if createdItem == nil {
				return PersistenceResult{}, fmt.Errorf("failed to create item %q", title)
			}
			itemIDs[item.LocalID] = createdItem.ItemID
			for _, chunkID := range item.SourceChunkIDs {
				if err := b.Repo.UpsertItemSource(ctx, createdItem.ItemID, args.DocumentID, chunkID, item.Description, 0.75); err != nil {
					return PersistenceResult{}, err
				}
			}
			created++
		}
		return PersistenceResult{Success: true, Message: fmt.Sprintf("Successfully persisted %d items", created)}, nil
	})
}
