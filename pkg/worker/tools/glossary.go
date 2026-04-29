package tools

import (
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type MemoryGlossary struct {
	mu      sync.RWMutex
	entries map[string]string // term -> definition
}

func NewMemoryGlossary() *MemoryGlossary {
	return &MemoryGlossary{entries: make(map[string]string)}
}

type GlossaryArgs struct {
	Action     string `json:"action" jsonschema:"enum=register,lookup,list,description=Action to perform on the glossary"`
	Term       string `json:"term,omitempty"`
	Definition string `json:"definition,omitempty"`
}

type GlossaryResult struct {
	Entries []GlossaryEntry `json:"entries,omitempty"`
	Message string          `json:"message"`
}

func NewGlossaryTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "manage_glossary",
		Description: "Manages domain-specific terminology and definitions for the current document. Use this to ensure consistent interpretation across large files.",
	}, func(ctx tool.Context, args GlossaryArgs) (GlossaryResult, error) {
		g := base.Glossary
		g.mu.Lock()
		defer g.mu.Unlock()

		switch args.Action {
		case "register":
			g.entries[args.Term] = args.Definition
			return GlossaryResult{Message: "Term registered: " + args.Term}, nil

		case "lookup":
			if def, ok := g.entries[args.Term]; ok {
				return GlossaryResult{
					Entries: []GlossaryEntry{{Term: args.Term, Definition: def}},
					Message: "Definition found",
				}, nil
			}
			return GlossaryResult{Message: "Term not found in glossary"}, nil

		case "list":
			var res []GlossaryEntry
			for t, d := range g.entries {
				res = append(res, GlossaryEntry{Term: t, Definition: d})
			}
			return GlossaryResult{Entries: res, Message: "Full glossary retrieved"}, nil

		default:
			return GlossaryResult{Message: "Invalid action"}, nil
		}
	})
}
