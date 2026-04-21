package eval

import (
	"fmt"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type Result struct {
	FixtureID string
	Stage     pipeline.StageName
	Passed    bool
	Failures  []string
}

type Runner struct{}

func (Runner) EvaluateGraphFixture(fixture Fixture, nodes []pipeline.SynthesizedNode, edges []pipeline.SynthesizedEdge) Result {
	result := Result{
		FixtureID: fixture.ID,
		Stage:     fixture.Stage,
		Passed:    true,
	}

	if len(nodes) < fixture.Expected.MinNodeCount {
		result.Failures = append(result.Failures, fmt.Sprintf("expected at least %d nodes, got %d", fixture.Expected.MinNodeCount, len(nodes)))
	}

	labelSet := map[string]bool{}
	for _, node := range nodes {
		labelSet[strings.TrimSpace(node.Label)] = true
	}
	for _, label := range fixture.Expected.RequiredLabels {
		if !labelSet[strings.TrimSpace(label)] {
			result.Failures = append(result.Failures, fmt.Sprintf("missing required label %q", label))
		}
	}

	edgeSet := map[string]bool{}
	for _, edge := range edges {
		edgeSet[edge.SourceLocalID+"\x00"+edge.TargetLocalID+"\x00"+edge.EdgeType] = true
	}
	for _, expect := range fixture.Expected.RequiredHierarchical {
		if !edgeSet[expect.Source+"\x00"+expect.Target+"\x00hierarchical"] {
			result.Failures = append(result.Failures, fmt.Sprintf("missing hierarchical edge %s -> %s", expect.Source, expect.Target))
		}
	}
	for _, expect := range fixture.Expected.RequiredTypedEdges {
		if !edgeSet[expect.Source+"\x00"+expect.Target+"\x00"+expect.EdgeType] {
			result.Failures = append(result.Failures, fmt.Sprintf("missing %s edge %s -> %s", expect.EdgeType, expect.Source, expect.Target))
		}
	}

	result.Passed = len(result.Failures) == 0
	return result
}
