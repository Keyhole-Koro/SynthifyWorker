package io

import "testing"

func TestInferDependencies_SubsectionsDependOnParent(t *testing.T) {
	sections := normalizeOutline([]string{
		"1 Introduction",
		"1.1 Definitions",
		"2 Usage",
	})

	deps := inferDependencies(sections)
	if len(deps) == 0 {
		t.Fatal("expected dependencies")
	}

	found := false
	for _, dep := range deps {
		if dep.TaskID == "task_2" && dep.DependsOn == "task_1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected subsection dependency, got %#v", deps)
	}
}

func TestSortByPriority_FoundationalSectionsComeFirst(t *testing.T) {
	sections := normalizeOutline([]string{
		"Usage Guide",
		"Introduction",
		"Deployment",
	})

	deps := inferDependencies(sections)
	priorities := sortByPriority(sections, deps)
	if len(priorities) != 3 {
		t.Fatalf("unexpected priorities length: %d", len(priorities))
	}
	if priorities[0] != "Introduction" {
		t.Fatalf("expected Introduction first, got %#v", priorities)
	}
}
