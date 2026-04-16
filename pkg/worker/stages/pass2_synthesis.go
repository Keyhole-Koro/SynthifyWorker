package stages

import (
	"context"
	"fmt"
	"sort"
	"strings"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type Pass2SynthesisStage struct {
	assembler workercontext.Assembler
}

func NewPass2SynthesisStage(assembler workercontext.Assembler) *Pass2SynthesisStage {
	return &Pass2SynthesisStage{assembler: assembler}
}

func (s *Pass2SynthesisStage) Name() pipeline.StageName { return pipeline.StagePass2Synthesis }

func (s *Pass2SynthesisStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	_ = s.assembler.ForPass2Normal(pctx)
	rootID := "doc_root"
	rootLabel := firstNonEmpty(pctx.DocumentBrief.Topic, pctx.Filename, "Document")
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
		return fmt.Errorf("pass2 synthesis produced no structure")
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
	pctx.SynthesizedNodes = mapToSortedNodes(nodes)
	pctx.SynthesizedEdges = edges
	return nil
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
