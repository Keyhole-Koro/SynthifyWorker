package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

var metricPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?%?`)

type Pass1ExtractionStage struct {
	assembler   workercontext.Assembler
	llm         workerllm.Client
	concurrency int
}

func NewPass1ExtractionStage(assembler workercontext.Assembler, llm workerllm.Client, concurrency int) *Pass1ExtractionStage {
	if concurrency <= 0 {
		concurrency = 5
	}
	return &Pass1ExtractionStage{assembler: assembler, llm: llm, concurrency: concurrency}
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
			bundle := s.assembler.ForPass1(pctx, chunk.ChunkIndex)
			nodes := extractRawNodes(chunk)
			if s.llm != nil {
				llmNodes, err := s.extractWithLLM(groupCtx, bundle, chunk.ChunkIndex)
				if err != nil {
					return err
				}
				nodes = llmNodes
			}
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

func (s *Pass1ExtractionStage) extractWithLLM(ctx context.Context, bundle workercontext.ContextBundle, chunkIndex int) ([]pipeline.RawNode, error) {
	resp, err := s.llm.GenerateStructured(ctx, workerllm.StructuredRequest{
		SystemPrompt: bundle.SystemPrompt + "\nSchema version: " + bundle.SchemaVersion + "\nReturn JSON: {\"nodes\":[{\"local_id\":\"n1\",\"label\":\"...\",\"level\":1,\"entity_type\":\"\",\"description\":\"...\",\"source_chunk_ids\":[\"c_000\"]}]}",
		UserPrompt:   bundle.UserPrompt,
		SourceFiles:  bundle.SourceFiles,
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Nodes []pipeline.RawNode `json:"nodes"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, err
	}
	var out []pipeline.RawNode
	expectedChunkID := fmt.Sprintf("c_%03d", chunkIndex)
	for _, node := range parsed.Nodes {
		node.LocalID = strings.TrimSpace(node.LocalID)
		node.Label = strings.TrimSpace(node.Label)
		node.Description = strings.TrimSpace(node.Description)
		if node.LocalID == "" || node.Label == "" {
			continue
		}
		if node.Level < 0 || node.Level > 3 {
			continue
		}
		node.SourceChunkIDs = uniqueNonEmpty(node.SourceChunkIDs)
		if len(node.SourceChunkIDs) != 1 || node.SourceChunkIDs[0] != expectedChunkID {
			continue
		}
		out = append(out, node)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("chunk %d produced no valid llm nodes", chunkIndex)
	}
	return out, nil
}

func extractRawNodes(chunk pipeline.Chunk) []pipeline.RawNode {
	var nodes []pipeline.RawNode
	if strings.TrimSpace(chunk.Heading) != "" {
		nodes = append(nodes, pipeline.RawNode{
			LocalID:        fmt.Sprintf("heading_%d", chunk.ChunkIndex),
			Label:          chunk.Heading,
			Level:          1,
			Description:    firstSentence(chunk.Text),
			SourceChunkIDs: []string{fmt.Sprintf("c_%03d", chunk.ChunkIndex)},
		})
	}
	for idx, match := range metricPattern.FindAllString(chunk.Text, -1) {
		nodes = append(nodes, pipeline.RawNode{
			LocalID:        fmt.Sprintf("metric_%d_%d", chunk.ChunkIndex, idx),
			Label:          match,
			Level:          3,
			EntityType:     "metric",
			Description:    "Extracted metric from source text",
			SourceChunkIDs: []string{fmt.Sprintf("c_%03d", chunk.ChunkIndex)},
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
				LocalID:        fmt.Sprintf("lead_%d", chunk.ChunkIndex),
				Label:          truncateLabel(line),
				Level:          2,
				Description:    line,
				SourceChunkIDs: []string{fmt.Sprintf("c_%03d", chunk.ChunkIndex)},
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
