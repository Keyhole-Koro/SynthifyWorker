package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type Pass2SynthesisStage struct {
	assembler workercontext.Assembler
	llm       workerllm.Client
}

func NewPass2SynthesisStage(assembler workercontext.Assembler, llm workerllm.Client) *Pass2SynthesisStage {
	return &Pass2SynthesisStage{assembler: assembler, llm: llm}
}

func (s *Pass2SynthesisStage) Name() pipeline.StageName { return pipeline.StagePass2Synthesis }

func (s *Pass2SynthesisStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	bundle := s.assembler.ForPass2Normal(pctx)
	if s.llm != nil {
		nodes, edges, err := s.synthesizeWithLLM(ctx, bundle, pctx)
		if err == nil {
			pctx.SynthesizedNodes = nodes
			pctx.SynthesizedEdges = edges
			return nil
		}
	}

	nodes, edges, err := synthesizeHeuristically(pctx)
	if err != nil {
		return err
	}
	pctx.SynthesizedNodes = nodes
	pctx.SynthesizedEdges = edges
	return nil
}

func (s *Pass2SynthesisStage) synthesizeWithLLM(ctx context.Context, bundle workercontext.ContextBundle, pctx *pipeline.PipelineContext) ([]pipeline.SynthesizedNode, []pipeline.SynthesizedEdge, error) {
	resp, err := s.llm.GenerateStructured(ctx, workerllm.StructuredRequest{
		SystemPrompt: bundle.SystemPrompt + `
Return JSON:
{
  "document_root": {
    "label": "...",
    "category": "concept",
    "description": "..."
  },
  "nodes": [
    {
      "local_id": "topic_a",
      "label": "...",
      "category": "concept|entity|claim|evidence|counter",
      "level": 1,
      "entity_type": "",
      "description": "...",
      "parent_local_id": "doc_root",
      "source_chunk_id": "c_000"
    }
  ],
  "edges": [
    {
      "source_local_id": "topic_a",
      "target_local_id": "claim_b",
      "edge_type": "hierarchical|supports|contradicts|measured_by|related_to",
      "description": "",
      "source_chunk_id": "c_000"
    }
  ]
}
Rules:
- local_id must be unique and must not be doc_root.
- parent_local_id must reference doc_root or another emitted node.
- Every node must preserve a valid source_chunk_id from the pass1 input.
- Prefer a compact hierarchy under doc_root; add non-hierarchical edges only when strongly supported.`,
		UserPrompt: buildPass2UserPrompt(bundle, pctx),
	})
	if err != nil {
		return nil, nil, err
	}

	var parsed struct {
		DocumentRoot struct {
			Label       string `json:"label"`
			Category    string `json:"category"`
			Description string `json:"description"`
		} `json:"document_root"`
		Nodes []struct {
			LocalID       string `json:"local_id"`
			Label         string `json:"label"`
			Category      string `json:"category"`
			Level         int    `json:"level"`
			EntityType    string `json:"entity_type"`
			Description   string `json:"description"`
			ParentLocalID string `json:"parent_local_id"`
			SourceChunkID string `json:"source_chunk_id"`
		} `json:"nodes"`
		Edges []struct {
			SourceLocalID string `json:"source_local_id"`
			TargetLocalID string `json:"target_local_id"`
			EdgeType      string `json:"edge_type"`
			Description   string `json:"description"`
			SourceChunkID string `json:"source_chunk_id"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, nil, err
	}

	rootLabel := firstNonEmpty(strings.TrimSpace(parsed.DocumentRoot.Label), documentTopic(pctx), pctx.Filename, "Document")
	rootDescription := firstNonEmpty(strings.TrimSpace(parsed.DocumentRoot.Description), firstSentence(pctx.RawText))
	nodesByID := map[string]pipeline.SynthesizedNode{
		"doc_root": {
			LocalID:     "doc_root",
			Label:       rootLabel,
			Category:    "concept",
			Level:       0,
			Description: rootDescription,
		},
	}

	validChunkIDs := validPass1ChunkIDs(pctx)
	for _, raw := range parsed.Nodes {
		localID := sanitizeLocalID(raw.LocalID)
		label := strings.TrimSpace(raw.Label)
		category := normalizeCategory(raw.Category)
		parentLocalID := sanitizeParentLocalID(raw.ParentLocalID)
		sourceChunkID := strings.TrimSpace(raw.SourceChunkID)
		if localID == "" || label == "" || localID == "doc_root" {
			continue
		}
		if _, exists := nodesByID[localID]; exists {
			continue
		}
		if !validChunkIDs[sourceChunkID] {
			continue
		}
		nodesByID[localID] = pipeline.SynthesizedNode{
			LocalID:       localID,
			Label:         label,
			Category:      category,
			Level:         clampLevel(raw.Level),
			EntityType:    normalizeEntityType(category, raw.EntityType),
			Description:   strings.TrimSpace(raw.Description),
			ParentLocalID: parentLocalID,
			SourceChunkID: sourceChunkID,
		}
	}
	if len(nodesByID) == 1 {
		return nil, nil, fmt.Errorf("pass2 synthesis produced no valid llm nodes")
	}

	for localID, node := range nodesByID {
		if localID == "doc_root" {
			continue
		}
		if node.ParentLocalID == "" || node.ParentLocalID == localID {
			node.ParentLocalID = "doc_root"
		}
		if _, ok := nodesByID[node.ParentLocalID]; !ok {
			node.ParentLocalID = "doc_root"
		}
		if node.Level == 0 {
			node.Level = 1
		}
		nodesByID[localID] = node
	}

	edges := make([]pipeline.SynthesizedEdge, 0, len(parsed.Edges)+len(nodesByID))
	seenEdges := map[string]bool{}
	for localID, node := range nodesByID {
		if localID == "doc_root" {
			continue
		}
		key := edgeKey(node.ParentLocalID, localID, "hierarchical")
		seenEdges[key] = true
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID: node.ParentLocalID,
			TargetLocalID: localID,
			EdgeType:      "hierarchical",
			SourceChunkID: node.SourceChunkID,
		})
	}

	for _, raw := range parsed.Edges {
		source := sanitizeParentLocalID(raw.SourceLocalID)
		target := sanitizeParentLocalID(raw.TargetLocalID)
		edgeType := normalizeEdgeType(raw.EdgeType)
		sourceChunkID := strings.TrimSpace(raw.SourceChunkID)
		if edgeType == "hierarchical" || source == "" || target == "" || source == target {
			continue
		}
		if !validChunkIDs[sourceChunkID] {
			continue
		}
		if _, ok := nodesByID[source]; !ok {
			continue
		}
		if _, ok := nodesByID[target]; !ok {
			continue
		}
		key := edgeKey(source, target, edgeType)
		if seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID: source,
			TargetLocalID: target,
			EdgeType:      edgeType,
			Description:   strings.TrimSpace(raw.Description),
			SourceChunkID: sourceChunkID,
		})
	}

	return mapToSortedNodes(nodesByID), sortEdges(edges), nil
}

func buildPass2UserPrompt(bundle workercontext.ContextBundle, pctx *pipeline.PipelineContext) string {
	var out strings.Builder
	if strings.TrimSpace(bundle.UserPrompt) != "" {
		out.WriteString("Outline:\n")
		out.WriteString(bundle.UserPrompt)
		out.WriteString("\n\n")
	}
	if pctx.DocumentBrief != nil {
		out.WriteString("Document brief:\n")
		out.WriteString(fmt.Sprintf("- topic: %s\n", pctx.DocumentBrief.Topic))
		out.WriteString(fmt.Sprintf("- claim_summary: %s\n", pctx.DocumentBrief.ClaimSummary))
		if len(pctx.DocumentBrief.Level01Hints) > 0 {
			out.WriteString("- level01_hints: " + strings.Join(pctx.DocumentBrief.Level01Hints, " | ") + "\n")
		}
		if len(pctx.DocumentBrief.Entities) > 0 {
			out.WriteString("- entities: " + strings.Join(pctx.DocumentBrief.Entities, " | ") + "\n")
		}
		out.WriteString("\n")
	}
	if len(pctx.SectionBriefs) > 0 {
		out.WriteString("Section briefs:\n")
		for idx, section := range pctx.SectionBriefs {
			out.WriteString(fmt.Sprintf("%d. heading=%s | topic=%s | candidates=%s | hints=%s\n",
				idx+1,
				section.Heading,
				section.Topic,
				strings.Join(section.NodeCandidates, ", "),
				section.ConnectionHints,
			))
		}
		out.WriteString("\n")
	}
	out.WriteString("Pass1 chunk extraction results:\n")
	keys := make([]int, 0, len(pctx.Pass1Results))
	for key := range pctx.Pass1Results {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for _, key := range keys {
		result := pctx.Pass1Results[key]
		out.WriteString(fmt.Sprintf("Chunk c_%03d:\n", result.ChunkIndex))
		for _, node := range result.Nodes {
			out.WriteString(fmt.Sprintf("- local_id=%s | label=%s | category=%s | level=%d | entity_type=%s | description=%s | source_chunk_id=%s\n",
				node.LocalID,
				node.Label,
				node.Category,
				node.Level,
				node.EntityType,
				node.Description,
				node.SourceChunkID,
			))
		}
	}
	return strings.TrimSpace(out.String())
}

func synthesizeHeuristically(pctx *pipeline.PipelineContext) ([]pipeline.SynthesizedNode, []pipeline.SynthesizedEdge, error) {
	rootID := "doc_root"
	rootLabel := firstNonEmpty(documentTopic(pctx), pctx.Filename, "Document")
	nodes := map[string]pipeline.SynthesizedNode{
		rootID: {
			LocalID:     rootID,
			Label:       rootLabel,
			Category:    "concept",
			Level:       0,
			Description: firstSentence(pctx.RawText),
		},
	}
	var edges []pipeline.SynthesizedEdge
	keys := make([]int, 0, len(pctx.Pass1Results))
	for key := range pctx.Pass1Results {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for _, key := range keys {
		result := pctx.Pass1Results[key]
		var parent string
		for _, rawNode := range result.Nodes {
			localID := fmt.Sprintf("p1_%d_%s", key, rawNode.LocalID)
			node := pipeline.SynthesizedNode{
				LocalID:       localID,
				Label:         rawNode.Label,
				Category:      normalizeCategory(rawNode.Category),
				Level:         clampLevel(rawNode.Level),
				EntityType:    normalizeEntityType(rawNode.Category, rawNode.EntityType),
				Description:   rawNode.Description,
				ParentLocalID: rootID,
				SourceChunkID: rawNode.SourceChunkID,
			}
			if node.Category == "concept" && parent == "" {
				parent = localID
				node.ParentLocalID = rootID
			} else if parent != "" && node.Level >= 2 {
				node.ParentLocalID = parent
			}
			nodes[localID] = node
		}
	}
	if len(nodes) == 1 {
		return nil, nil, fmt.Errorf("pass2 synthesis produced no structure")
	}
	for _, node := range nodes {
		if node.LocalID == rootID || node.ParentLocalID == "" {
			continue
		}
		edges = append(edges, pipeline.SynthesizedEdge{
			SourceLocalID: node.ParentLocalID,
			TargetLocalID: node.LocalID,
			EdgeType:      "hierarchical",
			SourceChunkID: node.SourceChunkID,
		})
	}
	return mapToSortedNodes(nodes), sortEdges(edges), nil
}

func mapToSortedNodes(nodes map[string]pipeline.SynthesizedNode) []pipeline.SynthesizedNode {
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]pipeline.SynthesizedNode, 0, len(keys))
	for _, key := range keys {
		out = append(out, nodes[key])
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

func validPass1ChunkIDs(pctx *pipeline.PipelineContext) map[string]bool {
	out := make(map[string]bool, len(pctx.Pass1Results))
	for _, result := range pctx.Pass1Results {
		for _, node := range result.Nodes {
			if strings.TrimSpace(node.SourceChunkID) != "" {
				out[node.SourceChunkID] = true
			}
		}
	}
	return out
}

func sanitizeLocalID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		if r == ' ' || r == '/' || r == '.' {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func sanitizeParentLocalID(value string) string {
	value = sanitizeLocalID(value)
	if value == "" {
		return "doc_root"
	}
	return value
}

func edgeKey(source, target, edgeType string) string {
	return source + "\x00" + target + "\x00" + edgeType
}

func normalizeEdgeType(edgeType string) string {
	switch strings.TrimSpace(edgeType) {
	case "hierarchical", "supports", "contradicts", "measured_by", "related_to":
		return strings.TrimSpace(edgeType)
	default:
		return "related_to"
	}
}

func documentTopic(pctx *pipeline.PipelineContext) string {
	if pctx.DocumentBrief == nil {
		return ""
	}
	return strings.TrimSpace(pctx.DocumentBrief.Topic)
}

func normalizeCategory(category string) string {
	switch strings.TrimSpace(category) {
	case "concept", "entity", "claim", "evidence", "counter":
		return category
	default:
		return "concept"
	}
}

func normalizeEntityType(category, entityType string) string {
	if category != "entity" {
		return ""
	}
	if strings.TrimSpace(entityType) == "" {
		return "unspecified"
	}
	return entityType
}

func clampLevel(level int) int {
	if level < 0 {
		return 0
	}
	if level > 3 {
		return 3
	}
	return level
}
