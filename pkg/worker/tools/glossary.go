package tools

import (
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type MemoryGlossary struct {
	mu      sync.RWMutex
	entries map[string]map[string]string // jobID -> term -> definition
}

var globalGlossary = &MemoryGlossary{entries: make(map[string]map[string]string)}

type GlossaryArgs struct {
	JobID      string `json:"job_id"`
	Action     string `json:"action" jsonschema:"enum=register,lookup,list,description=Action to perform on the glossary"`
	Term       string `json:"term,omitempty"`
	Definition string `json:"definition,omitempty"`
}

type GlossaryResult struct {
	Entries []GlossaryEntry `json:"entries,omitempty"`
	Message string          `json:"message"`
}

// NewGlossaryTool stores and retrieves per-job terminology definitions.
// Input schema: GlossaryArgs{job_id: string, action: string, term?: string, definition?: string}.
// Output schema: GlossaryResult{entries?: []GlossaryEntry, message: string}.
func NewGlossaryTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "manage_glossary",
		Description: "Manages domain-specific terminology and definitions for the current document. Use this to ensure consistent interpretation across large files.",
	}, func(ctx tool.Context, args GlossaryArgs) (GlossaryResult, error) {
		globalGlossary.mu.Lock()
		defer globalGlossary.mu.Unlock()

		if _, ok := globalGlossary.entries[args.JobID]; !ok {
			globalGlossary.entries[args.JobID] = make(map[string]string)
		}

		switch args.Action {
		case "register":
			globalGlossary.entries[args.JobID][args.Term] = args.Definition
			return GlossaryResult{Message: "Term registered: " + args.Term}, nil

		case "lookup":
			if def, ok := globalGlossary.entries[args.JobID][args.Term]; ok {
				return GlossaryResult{
					Entries: []GlossaryEntry{{Term: args.Term, Definition: def}},
					Message: "Definition found",
				}, nil
			}
			return GlossaryResult{Message: "Term not found in glossary"}, nil

		case "list":
			var res []GlossaryEntry
			for t, d := range globalGlossary.entries[args.JobID] {
				res = append(res, GlossaryEntry{Term: t, Definition: d})
			}
			return GlossaryResult{Entries: res, Message: "Full glossary retrieved"}, nil

		default:
			return GlossaryResult{Message: "Invalid action"}, nil
		}
	})
}
