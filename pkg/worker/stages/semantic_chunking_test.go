package stages

import (
	"context"
	"path/filepath"
	"testing"

	workercontext "github.com/synthify/backend/worker/pkg/worker/context"
	"github.com/synthify/backend/worker/pkg/worker/eval"
	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

func TestSemanticChunkingFixtures(t *testing.T) {
	fixtures, err := eval.LoadFixturesForStage(pipeline.StageSemanticChunking)
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("expected at least one semantic chunking fixture")
	}

	assembler := workercontext.NewDefaultAssembler(filepath.Join("..", "prompts"))

	for _, fixture := range fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			llm := &fakeLLMClient{structured: fixture.LLMResponse}
			stage := NewSemanticChunkingStage(assembler, llm)
			pctx := fixture.PipelineContext()
			if err := stage.Run(context.Background(), pctx); err != nil {
				t.Fatalf("run stage: %v", err)
			}

			if got := len(pctx.Chunks); got < fixture.Expected.MinChunkCount {
				t.Fatalf("expected at least %d chunks, got %d", fixture.Expected.MinChunkCount, got)
			}
			for _, heading := range fixture.Expected.RequiredChunkHeadings {
				if !hasChunkHeading(pctx.Chunks, heading) {
					t.Fatalf("expected chunk heading %q, got %+v", heading, pctx.Chunks)
				}
			}
			for _, outline := range fixture.Expected.RequiredOutline {
				if !containsString(pctx.Outline, outline) {
					t.Fatalf("expected outline %q, got %v", outline, pctx.Outline)
				}
			}
			if len(llm.lastStruct.SourceFiles) == 0 {
				t.Fatal("expected source files to be passed to llm")
			}
			if len(llm.lastStruct.SourceFiles[0].Content) == 0 {
				t.Fatal("expected source file content to be available to llm")
			}
		})
	}
}

func hasChunkHeading(chunks []pipeline.Chunk, heading string) bool {
	for _, chunk := range chunks {
		if chunk.Heading == heading {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
