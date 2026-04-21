package stages

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/eval"
	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type fakeLLMClient struct {
	structured json.RawMessage
}

func (f fakeLLMClient) GenerateStructured(ctx context.Context, req workerllm.StructuredRequest) (json.RawMessage, error) {
	return append(json.RawMessage(nil), f.structured...), nil
}

func (f fakeLLMClient) GenerateText(ctx context.Context, req workerllm.TextRequest) (string, error) {
	return "", nil
}

func TestPass2SynthesisFixtures(t *testing.T) {
	fixtures, err := eval.LoadFixturesForStage(pipeline.StagePass2Synthesis)
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("expected at least one pass2 fixture")
	}

	assembler := workercontext.NewDefaultAssembler(filepath.Join("..", "prompts"))
	runner := eval.Runner{}

	for _, fixture := range fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			stage := NewPass2SynthesisStage(assembler, fakeLLMClient{structured: fixture.LLMResponse})
			pctx := fixture.PipelineContext()
			if err := stage.Run(context.Background(), pctx); err != nil {
				t.Fatalf("run stage: %v", err)
			}

			result := runner.EvaluateGraphFixture(fixture, pctx.SynthesizedNodes, pctx.SynthesizedEdges)
			if !result.Passed {
				t.Fatalf("fixture failed: %v", result.Failures)
			}
		})
	}
}
