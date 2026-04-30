package memory

import (
	"strings"
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Entry struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type Glossary struct {
	mu      sync.RWMutex
	entries map[string]string
}

func NewGlossary() *Glossary {
	return &Glossary{entries: make(map[string]string)}
}

func (g *Glossary) RenderForPrompt() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Glossary\n")
	for term, def := range g.entries {
		sb.WriteString("- **")
		sb.WriteString(term)
		sb.WriteString("**: ")
		sb.WriteString(def)
		sb.WriteString("\n")
	}
	return sb.String()
}

type registerArgs struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

type registerResult struct {
	Message string `json:"message"`
}

func NewRegisterTool(g *Glossary) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "glossary_register",
		Description: "Registers a domain-specific term and its definition for consistent interpretation across the document.",
	}, func(ctx tool.Context, args registerArgs) (registerResult, error) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.entries[args.Term] = args.Definition
		return registerResult{Message: "Term registered: " + args.Term}, nil
	})
}

type lookupArgs struct {
	Term string `json:"term"`
}

type lookupResult struct {
	Entry   *Entry `json:"entry,omitempty"`
	Message string `json:"message"`
}

func NewLookupTool(g *Glossary) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "glossary_lookup",
		Description: "Looks up a term in the glossary.",
	}, func(ctx tool.Context, args lookupArgs) (lookupResult, error) {
		g.mu.RLock()
		defer g.mu.RUnlock()
		if def, ok := g.entries[args.Term]; ok {
			return lookupResult{Entry: &Entry{Term: args.Term, Definition: def}, Message: "Definition found"}, nil
		}
		return lookupResult{Message: "Term not found in glossary"}, nil
	})
}
