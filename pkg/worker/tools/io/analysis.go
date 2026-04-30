package io

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Keyhole-Koro/SynthifyShared/pipeline"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Dependency struct {
	TaskID    string `json:"task_id" jsonschema:"description=The ID of the task that has a dependency"`
	DependsOn string `json:"depends_on" jsonschema:"description=The ID of the prerequisite task"`
	Reason    string `json:"reason" jsonschema:"description=Why this dependency exists"`
}

type AnalysisArgs struct {
	Outline []string `json:"outline" jsonschema:"description=The list of section headings or chunk summaries"`
}

type AnalysisResult struct {
	Dependencies []Dependency `json:"dependencies"`
	Priorities   []string     `json:"priorities" jsonschema:"description=Recommended order of processing"`
}

type section struct {
	ID    string
	Title string
	Level int
	Index int
}

func NewAnalysisTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "analyze_dependencies",
		Description: "Analyzes document structure to identify logical dependencies between sections. Helps determine the optimal processing order.",
	}, func(ctx tool.Context, args AnalysisArgs) (AnalysisResult, error) {
		sections := normalizeOutline(args.Outline)
		dependencies := inferDependencies(sections)
		priorities := sortByPriority(sections, dependencies)
		return AnalysisResult{
			Dependencies: dependencies,
			Priorities:   priorities,
		}, nil
	})
}

func normalizeOutline(outline []string) []section {
	sections := make([]section, 0, len(outline))
	for i, raw := range outline {
		title := strings.TrimSpace(raw)
		if title == "" {
			title = fmt.Sprintf("section_%d", i+1)
		}
		sections = append(sections, section{
			ID:    fmt.Sprintf("task_%d", i+1),
			Title: title,
			Level: detectHeadingLevel(title),
			Index: i,
		})
	}
	return sections
}

func detectHeadingLevel(title string) int {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return 1
	}
	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level > 0 {
			return level
		}
	}
	if matches := pipeline.NumberedHeadingPattern.FindStringSubmatch(trimmed); len(matches) == 2 {
		return strings.Count(matches[1], ".") + 1
	}
	return 1
}

func inferDependencies(sections []section) []Dependency {
	var dependencies []Dependency
	for i, current := range sections {
		if parent := findParentSection(sections, i); parent != nil {
			dependencies = append(dependencies, Dependency{
				TaskID:    current.ID,
				DependsOn: parent.ID,
				Reason:    "subsection depends on its parent section",
			})
		}
		if prerequisite := findSemanticPrerequisite(sections, i); prerequisite != nil && !hasDependency(dependencies, current.ID, prerequisite.ID) {
			dependencies = append(dependencies, Dependency{
				TaskID:    current.ID,
				DependsOn: prerequisite.ID,
				Reason:    "section appears to rely on earlier background or definitions",
			})
		}
	}
	return dependencies
}

func findParentSection(sections []section, index int) *section {
	current := sections[index]
	for i := index - 1; i >= 0; i-- {
		if sections[i].Level < current.Level {
			return &sections[i]
		}
	}
	return nil
}

func findSemanticPrerequisite(sections []section, index int) *section {
	current := sections[index]
	title := strings.ToLower(current.Title)
	if isFoundational(title) || !needsPrerequisite(title) {
		return nil
	}
	bestIndex := -1
	bestDistance := len(sections) + 1
	for i := range sections {
		if i == index {
			continue
		}
		if !isFoundational(strings.ToLower(sections[i].Title)) {
			continue
		}
		distance := index - i
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance || (distance == bestDistance && i < bestIndex) {
			bestDistance = distance
			bestIndex = i
		}
	}
	if bestIndex >= 0 {
		return &sections[bestIndex]
	}
	if index > 0 {
		return &sections[index-1]
	}
	return nil
}

func isFoundational(title string) bool {
	keywords := []string{"overview", "introduction", "background", "definition", "definitions",
		"concept", "concepts", "terminology", "architecture", "context"}
	return containsAny(title, keywords)
}

func needsPrerequisite(title string) bool {
	keywords := []string{"usage", "workflow", "example", "examples", "implementation", "deployment",
		"configuration", "operation", "operations", "integration", "advanced",
		"troubleshooting", "how to", "guide"}
	return containsAny(title, keywords)
}

func containsAny(title string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(title, keyword) {
			return true
		}
	}
	return false
}

func hasDependency(dependencies []Dependency, taskID, dependsOn string) bool {
	for _, dep := range dependencies {
		if dep.TaskID == taskID && dep.DependsOn == dependsOn {
			return true
		}
	}
	return false
}

func sortByPriority(sections []section, dependencies []Dependency) []string {
	indegree := make(map[string]int, len(sections))
	graph := make(map[string][]string, len(sections))
	byID := make(map[string]section, len(sections))
	for _, s := range sections {
		indegree[s.ID] = 0
		byID[s.ID] = s
	}
	for _, dep := range dependencies {
		graph[dep.DependsOn] = append(graph[dep.DependsOn], dep.TaskID)
		indegree[dep.TaskID]++
	}

	var ready []section
	for _, s := range sections {
		if indegree[s.ID] == 0 {
			ready = append(ready, s)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Index < ready[j].Index })

	priorities := make([]string, 0, len(sections))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		priorities = append(priorities, current.Title)
		for _, nextID := range graph[current.ID] {
			indegree[nextID]--
			if indegree[nextID] == 0 {
				ready = append(ready, byID[nextID])
			}
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i].Index < ready[j].Index })
	}

	if len(priorities) != len(sections) {
		priorities = priorities[:0]
		for _, s := range sections {
			priorities = append(priorities, s.Title)
		}
	}
	return priorities
}
