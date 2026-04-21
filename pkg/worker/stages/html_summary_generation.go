package stages

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type SummaryRepository interface {
	UpdateNodeSummaryHTML(nodeID, summaryHTML string) bool
}

type HTMLSummaryGenerationStage struct {
	repo        SummaryRepository
	assembler   workercontext.Assembler
	llm         workerllm.Client
	concurrency int
}

func NewHTMLSummaryGenerationStage(repo SummaryRepository, assembler workercontext.Assembler, llm workerllm.Client, concurrency int) *HTMLSummaryGenerationStage {
	if concurrency <= 0 {
		concurrency = 10
	}
	return &HTMLSummaryGenerationStage{repo: repo, assembler: assembler, llm: llm, concurrency: concurrency}
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
			bundle := s.assembler.ForHTMLSummary(pctx, node.LocalID)
			summary := ""
			if s.llm != nil {
				generated, err := s.generateSummary(groupCtx, bundle, node, adjacency[node.LocalID], pctx)
				if err == nil && isAllowedHTMLSummary(generated) {
					summary = generated
				}
			}
			if summary == "" {
				summary = buildHTMLSummary(node, adjacency[node.LocalID], pctx)
			}
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

func (s *HTMLSummaryGenerationStage) generateSummary(ctx context.Context, bundle workercontext.ContextBundle, node pipeline.SynthesizedNode, neighbors []string, pctx *pipeline.PipelineContext) (string, error) {
	var related []string
	for _, localID := range neighbors {
		nodeID := pctx.NodeIDMap[localID]
		if nodeID == "" {
			continue
		}
		related = append(related, fmt.Sprintf("- %s (%s)", labelForLocalID(localID, pctx), nodeID))
	}
	userPrompt := strings.TrimSpace(bundle.UserPrompt + "\n" +
		"Label: " + node.Label + "\n" +
		"Description: " + node.Description + "\n" +
		"Related nodes:\n" + strings.Join(related, "\n"))
	return s.llm.GenerateText(ctx, workerllm.TextRequest{
		SystemPrompt: bundle.SystemPrompt + "\nSchema version: " + bundle.SchemaVersion + "\nUse only allowed tags: table, thead, tbody, tr, th, td, ul, ol, li, p, h3, h4, strong, em, a. Links must use only data-paper-id.",
		UserPrompt:   userPrompt,
	})
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

var htmlTagPattern = regexp.MustCompile(`(?i)</?([a-z0-9]+)\b`)

func isAllowedHTMLSummary(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	lowered := strings.ToLower(value)
	if strings.Contains(lowered, "href=") || strings.Contains(lowered, "onclick=") || strings.Contains(lowered, "style=") {
		return false
	}
	allowed := map[string]bool{
		"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
		"ul": true, "ol": true, "li": true, "p": true, "h3": true, "h4": true,
		"strong": true, "em": true, "a": true,
	}
	for _, match := range htmlTagPattern.FindAllStringSubmatch(lowered, -1) {
		if !allowed[match[1]] {
			return false
		}
	}
	return true
}
