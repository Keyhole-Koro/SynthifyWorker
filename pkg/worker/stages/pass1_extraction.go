package stages

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

var metricPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?%?`)

type Pass1ExtractionStage struct {
	assembler   workercontext.Assembler
	concurrency int
}

func NewPass1ExtractionStage(assembler workercontext.Assembler, concurrency int) *Pass1ExtractionStage {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &Pass1ExtractionStage{assembler: assembler, concurrency: concurrency}
}

func (s *Pass1ExtractionStage) Name() pipeline.StageName { return pipeline.StagePass1Extraction }

func (s *Pass1ExtractionStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	results := make(map[int]pipeline.Pass1ChunkResult, len(pctx.Chunks))
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, s.concurrency)
	for _, chunk := range pctx.Chunks {
		chunk := chunk
		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			_ = s.assembler.ForPass1(pctx, chunk.ChunkIndex)
			nodes := extractRawNodes(chunk)
			if len(nodes) == 0 {
				return fmt.Errorf("chunk %d produced no nodes", chunk.ChunkIndex)
			}
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			default:
			}
			mu.Lock()
			results[chunk.ChunkIndex] = pipeline.Pass1ChunkResult{ChunkIndex: chunk.ChunkIndex, Nodes: nodes}
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	pctx.Pass1Results = results
	return nil
}

func extractRawNodes(chunk pipeline.Chunk) []pipeline.RawNode {
	var nodes []pipeline.RawNode
	if strings.TrimSpace(chunk.Heading) != "" {
		nodes = append(nodes, pipeline.RawNode{
			LocalID:       fmt.Sprintf("heading_%d", chunk.ChunkIndex),
			Label:         chunk.Heading,
			Category:      "concept",
			Level:         1,
			Description:   firstSentence(chunk.Text),
			SourceChunkID: fmt.Sprintf("c_%03d", chunk.ChunkIndex),
		})
	}
	for idx, match := range metricPattern.FindAllString(chunk.Text, -1) {
		nodes = append(nodes, pipeline.RawNode{
			LocalID:       fmt.Sprintf("metric_%d_%d", chunk.ChunkIndex, idx),
			Label:         match,
			Category:      "entity",
			Level:         3,
			EntityType:    "metric",
			Description:   "Extracted metric from source text",
			SourceChunkID: fmt.Sprintf("c_%03d", chunk.ChunkIndex),
		})
	}
	lines := strings.Split(chunk.Text, "\n")
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx == 0 && chunk.Heading == "" {
			nodes = append(nodes, pipeline.RawNode{
				LocalID:       fmt.Sprintf("lead_%d", chunk.ChunkIndex),
				Label:         truncateLabel(line),
				Category:      "claim",
				Level:         2,
				Description:   line,
				SourceChunkID: fmt.Sprintf("c_%03d", chunk.ChunkIndex),
			})
			break
		}
	}
	return nodes
}

func truncateLabel(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 32 {
		return string(runes[:32])
	}
	return string(runes)
}

func extractMetrics(text string) []string {
	return uniqueNonEmpty(metricPattern.FindAllString(text, -1))
}
