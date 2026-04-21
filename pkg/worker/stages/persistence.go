package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type GraphRepository interface {
	GetWorkspaceRootNodeID(graphID string) (string, bool)
	SaveDocumentChunks(documentID string, chunks []*domain.DocumentChunk) error
	CreateStructuredNode(graphID, label string, level int, entityType, description, summaryHTML, createdBy string) *domain.Node
	CreateEdge(graphID, sourceNodeID, targetNodeID, edgeType, description string) *domain.Edge
	UpsertNodeSource(nodeID, documentID, chunkID, sourceText string, confidence float64) error
	UpsertEdgeSource(edgeID, documentID, chunkID, sourceText string, confidence float64) error
}

type PersistenceStage struct {
	repo GraphRepository
}

func NewPersistenceStage(repo GraphRepository) *PersistenceStage {
	return &PersistenceStage{repo: repo}
}

func (s *PersistenceStage) Name() pipeline.StageName { return pipeline.StagePersistence }

func (s *PersistenceStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	pctx.NodeIDMap = make(map[string]string, len(pctx.SynthesizedNodes))
	workspaceRootID, _ := s.repo.GetWorkspaceRootNodeID(pctx.GraphID)
	chunks := make([]*domain.DocumentChunk, 0, len(pctx.Chunks))
	chunkTextByID := map[string]string{}
	for _, chunk := range pctx.Chunks {
		chunkID := fmt.Sprintf("chk_%s_%03d", pctx.DocumentID, chunk.ChunkIndex)
		chunks = append(chunks, &domain.DocumentChunk{
			ChunkID:    chunkID,
			DocumentID: pctx.DocumentID,
			Heading:    chunk.Heading,
			Text:       chunk.Text,
		})
		chunkTextByID[fmt.Sprintf("c_%03d", chunk.ChunkIndex)] = chunk.Text
	}
	if err := s.repo.SaveDocumentChunks(pctx.DocumentID, chunks); err != nil {
		return err
	}
	for _, node := range pctx.SynthesizedNodes {
		created := s.repo.CreateStructuredNode(
			pctx.GraphID,
			node.Label,
			node.Level,
			node.EntityType,
			node.Description,
			node.SummaryHTML,
			"worker",
		)
		if created == nil {
			return fmt.Errorf("failed to persist node %s", node.LocalID)
		}
		pctx.NodeIDMap[node.LocalID] = created.NodeID
		for _, chunkID := range node.SourceChunkIDs {
			if chunkText := strings.TrimSpace(chunkTextByID[chunkID]); chunkText != "" {
				if err := s.repo.UpsertNodeSource(created.NodeID, pctx.DocumentID, chunkID, chunkText, 1); err != nil {
					return err
				}
			}
		}
	}
	for _, edge := range pctx.SynthesizedEdges {
		sourceID := pctx.NodeIDMap[edge.SourceLocalID]
		targetID := pctx.NodeIDMap[edge.TargetLocalID]
		if sourceID == "" || targetID == "" {
			continue
		}
		created := s.repo.CreateEdge(pctx.GraphID, sourceID, targetID, edge.EdgeType, edge.Description)
		if created == nil {
			return fmt.Errorf("failed to persist edge %s->%s", edge.SourceLocalID, edge.TargetLocalID)
		}
		for _, chunkID := range edge.SourceChunkIDs {
			if chunkText := strings.TrimSpace(chunkTextByID[chunkID]); chunkText != "" {
				if err := s.repo.UpsertEdgeSource(created.EdgeID, pctx.DocumentID, chunkID, chunkText, 1); err != nil {
					return err
				}
			}
		}
	}
	if workspaceRootID != "" {
		if docRootID := pctx.NodeIDMap["doc_root"]; docRootID != "" && docRootID != workspaceRootID {
			if s.repo.CreateEdge(pctx.GraphID, workspaceRootID, docRootID, "hierarchical", "") == nil {
				return fmt.Errorf("failed to attach document root %s to workspace root", docRootID)
			}
		}
	}
	return nil
}
