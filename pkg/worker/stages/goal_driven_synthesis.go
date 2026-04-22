package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type GoalDrivenSynthesisStage struct {
	llm workerllm.Client
}

func NewGoalDrivenSynthesisStage(llm workerllm.Client) *GoalDrivenSynthesisStage {
	return &GoalDrivenSynthesisStage{llm: llm}
}

func (s *GoalDrivenSynthesisStage) Name() pipeline.StageName {
	return pipeline.StageGoalDrivenSynthesis
}

func (s *GoalDrivenSynthesisStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	if s.llm != nil {
		nodes, edges, err := s.synthesizeWithLLM(ctx, pctx)
		if err == nil && len(nodes) > 0 {
			pctx.SynthesizedNodes = nodes
			pctx.SynthesizedEdges = edges
			return nil
		}
	}

	nodes, edges, err := synthesizeGoalDrivenHeuristically(pctx)
	if err != nil {
		return err
	}
	pctx.SynthesizedNodes = nodes
	pctx.SynthesizedEdges = edges
	return nil
}

func (s *GoalDrivenSynthesisStage) synthesizeWithLLM(ctx context.Context, pctx *pipeline.PipelineContext) ([]pipeline.SynthesizedNode, []pipeline.SynthesizedEdge, error) {
	resp, err := s.llm.GenerateStructured(ctx, workerllm.StructuredRequest{
		SystemPrompt: `You are building a compact document graph.
Return JSON:
{
  "document_root": {"label": "...", "description": "..."},
  "nodes": [
    {
      "local_id": "topic_a",
      "label": "...",
      "level": 1,
      "description": "...",
      "parent_local_id": "doc_root",
      "source_chunk_ids": ["c_000"]
    }
  ],
  "edges": [
    {
      "source_local_id": "topic_a",
      "target_local_id": "claim_b",
      "edge_type": "hierarchical|supports|contradicts|measured_by|related_to",
      "description": "",
      "source_chunk_ids": ["c_000"]
    }
  ]
}
Rules:
- Build only nodes strongly grounded in the chunks.
- Use doc_root as the single root.
- Keep the graph compact.
- Every node and edge must include valid source_chunk_ids.`,
		UserPrompt: buildGoalDrivenPrompt(pctx),
	})
	if err != nil {
		return nil, nil, err
	}

	var parsed struct {
		DocumentRoot struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"document_root"`
		Nodes []struct {
			LocalID        string   `json:"local_id"`
			Label          string   `json:"label"`
			Level          int      `json:"level"`
			Description    string   `json:"description"`
			ParentLocalID  string   `json:"parent_local_id"`
			SourceChunkIDs []string `json:"source_chunk_ids"`
		} `json:"nodes"`
		Edges []struct {
			SourceLocalID  string   `json:"source_local_id"`
			TargetLocalID  string   `json:"target_local_id"`
			EdgeType       string   `json:"edge_type"`
			Description    string   `json:"description"`
			SourceChunkIDs []string `json:"source_chunk_ids"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, nil, err
	}

	validChunkIDs := validChunkIDsFromChunks(pctx)
	nodesByID := map[string]pipeline.SynthesizedNode{
		"doc_root": {
			LocalID:     "doc_root",
			Label:       firstNonEmpty(strings.TrimSpace(parsed.DocumentRoot.Label), documentTopicFromBrief(pctx), pctx.Filename, "Document"),
			Level:       0,
			Description: firstNonEmpty(strings.TrimSpace(parsed.DocumentRoot.Description), firstSentence(pctx.RawText)),
		},
	}

	for _, raw := range parsed.Nodes {
		localID := sanitizeLocalID(raw.LocalID)
		if localID == "" || localID == "doc_root" {
			continue
		}
		sourceChunkIDs := filterValidChunkIDs(raw.SourceChunkIDs, validChunkIDs)
		if len(sourceChunkIDs) == 0 {
			continue
		}
		nodesByID[localID] = pipeline.SynthesizedNode{
			LocalID:        localID,
			Label:          strings.TrimSpace(raw.Label),
			Level:          clampLevel(raw.Level),
			Description:    strings.TrimSpace(raw.Description),
			ParentLocalID:  sanitizeParentLocalID(raw.ParentLocalID),
			SourceChunkIDs: sourceChunkIDs,
		}
	}
	if len(nodesByID) == 1 {
		return nil, nil, fmt.Errorf("goal-driven synthesis produced no valid nodes")
	}

	var edges []pipeline.SynthesizedEdge
	seen := map[string]bool{}
	for localID, node := range nodesByID {
		if localID == "doc_root" {
			continue
		}
		parent := node.ParentLocalID
		if parent == "" || parent == localID {
			parent = "doc_root"
		}
		if _, ok := nodesByID[parent]; !ok {
			parent = "doc_root"
		}
		node.ParentLocalID = parent
		nodesByID[localID] = node
		key := edgeKey(parent, localID, "hierarchical")
		seen[key] = true
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID:  parent,
			TargetLocalID:  localID,
			EdgeType:       "hierarchical",
			SourceChunkIDs: append([]string(nil), node.SourceChunkIDs...),
		})
	}

	for _, raw := range parsed.Edges {
		source := sanitizeParentLocalID(raw.SourceLocalID)
		target := sanitizeParentLocalID(raw.TargetLocalID)
		edgeType := normalizeEdgeType(raw.EdgeType)
		if edgeType == "hierarchical" || source == "" || target == "" || source == target {
			continue
		}
		if _, ok := nodesByID[source]; !ok {
			continue
		}
		if _, ok := nodesByID[target]; !ok {
			continue
		}
		sourceChunkIDs := filterValidChunkIDs(raw.SourceChunkIDs, validChunkIDs)
		if len(sourceChunkIDs) == 0 {
			continue
		}
		key := edgeKey(source, target, edgeType)
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID:  source,
			TargetLocalID:  target,
			EdgeType:       edgeType,
			Description:    strings.TrimSpace(raw.Description),
			SourceChunkIDs: sourceChunkIDs,
		})
	}

	return mapToSortedNodes(nodesByID), sortEdges(edges), nil
}

func synthesizeGoalDrivenHeuristically(pctx *pipeline.PipelineContext) ([]pipeline.SynthesizedNode, []pipeline.SynthesizedEdge, error) {
	rootLabel := firstNonEmpty(documentTopicFromBrief(pctx), pctx.Filename, "Document")
	nodes := []pipeline.SynthesizedNode{{
		LocalID:     "doc_root",
		Label:       rootLabel,
		Level:       0,
		Description: firstSentence(pctx.RawText),
	}}
	var edges []pipeline.SynthesizedEdge

	for _, chunk := range pctx.Chunks {
		chunkID := fmt.Sprintf("c_%03d", chunk.ChunkIndex)
		topicID := fmt.Sprintf("chunk_%03d", chunk.ChunkIndex)
		label := firstNonEmpty(strings.TrimSpace(chunk.Heading), truncateLabel(firstSentence(chunk.Text)), fmt.Sprintf("Chunk %d", chunk.ChunkIndex+1))
		description := firstSentence(chunk.Text)
		nodes = append(nodes, pipeline.SynthesizedNode{
			LocalID:        topicID,
			Label:          label,
			Level:          1,
			Description:    description,
			ParentLocalID:  "doc_root",
			SourceChunkIDs: []string{chunkID},
		})
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID:  "doc_root",
			TargetLocalID:  topicID,
			EdgeType:       "hierarchical",
			SourceChunkIDs: []string{chunkID},
		})

		for idx, metric := range extractMetrics(chunk.Text) {
			metricID := fmt.Sprintf("%s_metric_%d", topicID, idx)
			nodes = append(nodes, pipeline.SynthesizedNode{
				LocalID:        metricID,
				Label:          metric,
				Level:          3,
				Description:    "Extracted metric from source text",
				ParentLocalID:  topicID,
				SourceChunkIDs: []string{chunkID},
			})
			edges = append(edges, pipeline.SynthesizedEdge{
				SourceLocalID:  topicID,
				TargetLocalID:  metricID,
				EdgeType:       "measured_by",
				SourceChunkIDs: []string{chunkID},
			})
		}
	}

	if len(nodes) == 1 {
		return nil, nil, fmt.Errorf("goal-driven synthesis produced no structure")
	}
	return nodes, sortEdges(edges), nil
}

func buildGoalDrivenPrompt(pctx *pipeline.PipelineContext) string {
	var out strings.Builder
	if pctx.DocumentBrief != nil {
		out.WriteString("Document brief:\n")
		out.WriteString(fmt.Sprintf("- topic: %s\n", pctx.DocumentBrief.Topic))
		out.WriteString(fmt.Sprintf("- claim_summary: %s\n", pctx.DocumentBrief.ClaimSummary))
		if len(pctx.DocumentBrief.Level01Hints) > 0 {
			out.WriteString("- level01_hints: " + strings.Join(pctx.DocumentBrief.Level01Hints, " | ") + "\n")
		}
		out.WriteString("\n")
	}
	out.WriteString("Chunks:\n")
	for _, chunk := range pctx.Chunks {
		out.WriteString(fmt.Sprintf("Chunk c_%03d\n", chunk.ChunkIndex))
		if strings.TrimSpace(chunk.Heading) != "" {
			out.WriteString("Heading: " + strings.TrimSpace(chunk.Heading) + "\n")
		}
		out.WriteString("Text:\n" + strings.TrimSpace(chunk.Text) + "\n\n")
	}
	return strings.TrimSpace(out.String())
}

func validChunkIDsFromChunks(pctx *pipeline.PipelineContext) map[string]bool {
	out := make(map[string]bool, len(pctx.Chunks))
	for _, chunk := range pctx.Chunks {
		out[fmt.Sprintf("c_%03d", chunk.ChunkIndex)] = true
	}
	return out
}

func documentTopicFromBrief(pctx *pipeline.PipelineContext) string {
	if pctx.DocumentBrief == nil {
		return ""
	}
	return strings.TrimSpace(pctx.DocumentBrief.Topic)
}

func sanitizeLocalID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func sanitizeParentLocalID(value string) string {
	value = sanitizeLocalID(value)
	if value == "" {
		return "doc_root"
	}
	return value
}

func clampLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 3 {
		return 3
	}
	return level
}

func filterValidChunkIDs(values []string, valid map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !valid[value] || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeEdgeType(edgeType string) string {
	switch strings.TrimSpace(strings.ToLower(edgeType)) {
	case "supports", "contradicts", "measured_by", "related_to":
		return strings.TrimSpace(strings.ToLower(edgeType))
	default:
		return "related_to"
	}
}

func edgeKey(source, target, edgeType string) string {
	return source + "|" + target + "|" + edgeType
}

func mapToSortedNodes(nodesByID map[string]pipeline.SynthesizedNode) []pipeline.SynthesizedNode {
	keys := make([]string, 0, len(nodesByID))
	for key := range nodesByID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]pipeline.SynthesizedNode, 0, len(keys))
	for _, key := range keys {
		out = append(out, nodesByID[key])
	}
	return out
}

func sortEdges(edges []pipeline.SynthesizedEdge) []pipeline.SynthesizedEdge {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].SourceLocalID != edges[j].SourceLocalID {
			return edges[i].SourceLocalID < edges[j].SourceLocalID
		}
		if edges[i].TargetLocalID != edges[j].TargetLocalID {
			return edges[i].TargetLocalID < edges[j].TargetLocalID
		}
		return edges[i].EdgeType < edges[j].EdgeType
	})
	return edges
}
