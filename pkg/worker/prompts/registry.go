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
	HTMLSummary      = "html_summary"
)

var registry = map[string]Spec{
	SemanticChunking: {Name: SemanticChunking, PromptVersion: "v1", SchemaVersion: "chunk_schema_v1"},
	BriefGeneration:  {Name: BriefGeneration, PromptVersion: "v1", SchemaVersion: "brief_schema_v1"},
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
