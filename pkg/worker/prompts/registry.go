package prompts

import (
	"fmt"
	"path/filepath"
)

type Spec struct {
	Name          string
	PromptVersion string
	SchemaVersion string
}

const (
	SemanticChunking = "semantic_chunking"
	BriefGeneration  = "brief_generation"
	Pass1Extraction  = "pass1_extraction"
	Pass2Synthesis   = "pass2_synthesis"
	HTMLSummary      = "html_summary"
)

var registry = map[string]Spec{
	SemanticChunking: {Name: SemanticChunking, PromptVersion: "v1", SchemaVersion: "chunk_schema_v1"},
	BriefGeneration:  {Name: BriefGeneration, PromptVersion: "v1", SchemaVersion: "brief_schema_v1"},
	Pass1Extraction:  {Name: Pass1Extraction, PromptVersion: "v1", SchemaVersion: "pass1_nodes_v1"},
	Pass2Synthesis:   {Name: Pass2Synthesis, PromptVersion: "v1", SchemaVersion: "pass2_graph_v1"},
	HTMLSummary:      {Name: HTMLSummary, PromptVersion: "v1", SchemaVersion: "html_summary_v1"},
}

func Lookup(name string) (Spec, bool) {
	spec, ok := registry[name]
	return spec, ok
}

func MustLookup(name string) Spec {
	spec, ok := Lookup(name)
	if !ok {
		panic(fmt.Sprintf("unknown prompt %q", name))
	}
	return spec
}

func Path(baseDir, name string) string {
	spec := MustLookup(name)
	return filepath.Join(baseDir, spec.Name, spec.PromptVersion+".txt")
}
