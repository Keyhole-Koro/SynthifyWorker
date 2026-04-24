package agents

import (
	"context"

	"github.com/synthify/backend/worker/pkg/worker/tools"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

type Orchestrator struct {
	agent agent.Agent
}

func NewOrchestrator(m model.LLM, base *tools.BaseContext) (*Orchestrator, error) {
	chunking, err := tools.NewChunkingTool(base)
	if err != nil {
		return nil, err
	}
	synthesis, err := tools.NewSynthesisTool(base)
	if err != nil {
		return nil, err
	}
	persistence, err := tools.NewPersistenceTool(base)
	if err != nil {
		return nil, err
	}
	journal, err := tools.NewJournalTool()
	if err != nil {
		return nil, err
	}
	analysis, err := tools.NewAnalysisTool()
	if err != nil {
		return nil, err
	}

	a, err := llmagent.New(llmagent.Config{
		Name: "orchestrator",
		Model: m,
		Instruction: `You are the Senior Knowledge Engineer for Synthify.
Your mission is to transform raw information into a highly navigable knowledge tree.

Core Expertise (Your Assembly Logic):
- Semantic Chunking: Identify conceptual boundaries. Look for shifts in topic, tone, or perspective.
- Knowledge Synthesis: Do not just summarize. Reconstruct information as a tree of interdependent concepts, claims, and evidence. 
- Recursive Refinement: If a section is complex, break it down further. If information is missing, note it.

Operational Strategy:
1. Document Analysis: Examine the document to determine its nature (e.g., technical manual vs. legal contract).
2. Checklist Initialization: Use 'manage_job_checklist' to plan the decomposition of the document.
3. Execution & Adaptation:
   - For each section, formulate specific instructions for tools based on the document's logic.
   - Use 'get_next_pending' to manage the flow.
4. Final Review: Use 'persist_knowledge_tree' only when the resulting structure meets your high standards of clarity and depth.

You define the 'how'. You are not bound by fixed templates. Use your judgment to decide the best prompt assembly for each tool call.`,
		Tools: []tool.Tool{chunking, synthesis, persistence, journal, analysis},
	})
	if err != nil {
		return nil, err
	}

	return &Orchestrator{agent: a}, nil
}

func (o *Orchestrator) ProcessDocument(ctx context.Context, jobID, documentID, rawText string) (string, error) {
	// ADK Agent.Run returns an iterator of events.
	// For now, we provide a placeholder as full event processing requires a session and runner.
	return "Orchestration started (ADK transition in progress)", nil
}
