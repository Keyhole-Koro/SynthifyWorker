package agents

import (
	"context"
	"encoding/json"
	"time"

	"github.com/synthify/backend/worker/pkg/worker/tools"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

type Orchestrator struct {
	agent agent.Agent
}

// ToolLogger matches the repository interface for logging tool calls.
type ToolLogger interface {
	LogToolCall(ctx context.Context, jobID, toolName, inputJSON, outputJSON string, durationMs int64) error
}

func NewOrchestrator(m model.LLM, base *tools.BaseContext, repo any) (*Orchestrator, error) {
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
	glossary, err := tools.NewGlossaryTool()
	if err != nil {
		return nil, err
	}
	critique, err := tools.NewCritiqueTool()
	if err != nil {
		return nil, err
	}
	search, err := tools.NewSearchTool(base)
	if err != nil {
		return nil, err
	}
	merge, err := tools.NewMergeTool()
	if err != nil {
		return nil, err
	}
	tables, err := tools.NewTableTool()
	if err != nil {
		return nil, err
	}
	repair, err := tools.NewRepairTool()
	if err != nil {
		return nil, err
	}
	extraction, err := tools.NewExtractionTool()
	if err != nil {
		return nil, err
	}
	briefing, err := tools.NewBriefTool()
	if err != nil {
		return nil, err
	}
	summary, err := tools.NewSummaryTool()
	if err != nil {
		return nil, err
	}

	a, err := llmagent.New(llmagent.Config{
		Name:  "orchestrator",
		Model: m,
		Instruction: `You are the Lead Knowledge Architect for Synthify.
Your mission is to build a flawless knowledge tree from raw document data.

Core Engineering Workflow:
1. Preparation: Determine document nature. Use 'repair_encoding' if text is garbled.
2. Planning: Use 'manage_job_checklist' and 'analyze_dependencies' to map out the extraction.
3. Intelligence: Generate a 'generate_brief' to understand the core themes. This is your master blueprint.
4. Intelligent Execution (Context-Aware):
   - When calling 'goal_driven_synthesis', always pass the 'document_brief' and relevant 'glossary' entries.
   - If the current section references past topics, use 'semantic_search' to refresh your memory.
   - For each major concept, check the 'parent_structure' to ensure the new item is grafted into the tree correctly.
   - If you encounter a table, use 'extract_table_data' to preserve its logic.
5. Content Refinement: Use 'generate_html_summary' for each key item.
6. Quality Control:
   - Use 'quality_critique' to audit your work against the original source.
   - Use 'deduplicate_and_merge' to resolve redundant concepts across chapters.
7. Finalization: 'persist_knowledge_tree' only when the tree is architecturally sound.

You are self-correcting and possess a 'Working Memory'. Always lookup specialized terms and maintain the global context from the blueprint.`,
		Tools: []tool.Tool{chunking, synthesis, persistence, journal, analysis, glossary, critique, search, merge, tables, repair, extraction, briefing, summary},
		AfterToolCallbacks: []llmagent.AfterToolCallback{
			func(ctx tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
				start := time.Now()

				// Extract JobID from args or context (best effort)
				jobID, _ := args["job_id"].(string)

				argJSON, _ := json.Marshal(args)
				resJSON, _ := json.Marshal(result)
				if err != nil {
					resJSON, _ = json.Marshal(map[string]string{"error": err.Error()})
				}

				if logger, ok := repo.(ToolLogger); ok && jobID != "" {
					_ = logger.LogToolCall(context.Background(), jobID, t.Name(), string(argJSON), string(resJSON), time.Since(start).Milliseconds())
				}

				return result, err
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &Orchestrator{agent: a}, nil
}

func (o *Orchestrator) Agent() agent.Agent {
	if o == nil {
		return nil
	}
	return o.agent
}

func (o *Orchestrator) ProcessDocument(ctx context.Context, jobID, documentID, rawText string) (string, error) {
	// ADK Agent.Run returns an iterator of events.
	// For now, we provide a placeholder as full event processing requires a session and runner.
	return "Orchestration started (ADK transition in progress)", nil
}
