package stages

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type SummaryRepository interface {
	UpdateNodeSummaryHTML(nodeID, summaryHTML string) bool
}

type HTMLSummaryGenerationStage struct {
	repo        SummaryRepository
	assembler   workercontext.Assembler
	concurrency int
}

func NewHTMLSummaryGenerationStage(repo SummaryRepository, assembler workercontext.Assembler, concurrency int) *HTMLSummaryGenerationStage {
	if concurrency <= 0 {
		concurrency = 10
	}
	return &HTMLSummaryGenerationStage{repo: repo, assembler: assembler, concurrency: concurrency}
}

func (s *HTMLSummaryGenerationStage) Name() pipeline.StageName {
	return pipeline.StageHTMLSummaryGeneration
}

func (s *HTMLSummaryGenerationStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	adjacency := buildAdjacency(pctx.SynthesizedEdges)
	group, groupCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, s.concurrency)
	var mu sync.Mutex
	for _, node := range pctx.SynthesizedNodes {
		node := node
		if node.LocalID == "doc_root" {
			continue
		}
		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			_ = s.assembler.ForHTMLSummary(pctx, node.LocalID)
			summary := buildHTMLSummary(node, adjacency[node.LocalID], pctx)
			if summary == "" {
				return nil
			}
			nodeID := pctx.NodeIDMap[node.LocalID]
			if nodeID == "" {
				return nil
			}
			mu.Lock()
			s.repo.UpdateNodeSummaryHTML(nodeID, summary)
			mu.Unlock()
			return groupCtx.Err()
		})
	}
	return group.Wait()
}

func buildAdjacency(edges []pipeline.SynthesizedEdge) map[string][]string {
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.SourceLocalID] = append(adj[edge.SourceLocalID], edge.TargetLocalID)
		adj[edge.TargetLocalID] = append(adj[edge.TargetLocalID], edge.SourceLocalID)
	}
	return adj
}

func buildHTMLSummary(node pipeline.SynthesizedNode, neighbors []string, pctx *pipeline.PipelineContext) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("<p>%s</p>", escapeHTML(node.Description)))
	sort.Strings(neighbors)
	if len(neighbors) > 0 {
		var links []string
		for _, localID := range neighbors {
			nodeID := pctx.NodeIDMap[localID]
			if nodeID == "" {
				continue
			}
			links = append(links, fmt.Sprintf(`<li><a data-paper-id="%s">%s</a></li>`, escapeHTML(nodeID), escapeHTML(labelForLocalID(localID, pctx))))
		}
		if len(links) > 0 {
			parts = append(parts, "<ul>"+strings.Join(links, "")+"</ul>")
		}
	}
	return strings.Join(parts, "")
}

func labelForLocalID(localID string, pctx *pipeline.PipelineContext) string {
	for _, node := range pctx.SynthesizedNodes {
		if node.LocalID == localID {
			return node.Label
		}
	}
	return localID
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(value)
}
