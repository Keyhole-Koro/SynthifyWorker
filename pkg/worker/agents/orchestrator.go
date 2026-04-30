package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/synthify/backend/worker/pkg/worker/tools/base"
	toolsio "github.com/synthify/backend/worker/pkg/worker/tools/io"
	"github.com/synthify/backend/worker/pkg/worker/tools/memory"
	"github.com/synthify/backend/worker/pkg/worker/tools/process"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

type Orchestrator struct {
	agent agent.Agent
}

// ToolLogger matches the repository interface for logging tool calls.
type ToolLogger interface {
	LogToolCall(ctx context.Context, jobID, toolName, inputJSON, outputJSON string, durationMs int64) error
}

func NewOrchestrator(m model.LLM, b *base.Context, repo any) (*Orchestrator, error) {
	glossary := memory.NewGlossary()
	journal := memory.NewJournal()
	brief := memory.NewBrief()

	b.Memories = []base.PromptMemory{brief, glossary, journal}

	chunking, err := toolsio.NewChunkingTool(b)
	if err != nil {
		return nil, err
	}
	synthesis, err := process.NewSynthesisTool(b)
	if err != nil {
		return nil, err
	}
	persistence, err := toolsio.NewPersistenceTool(b)
	if err != nil {
		return nil, err
	}
	journalAdd, err := memory.NewAddTaskTool(journal)
	if err != nil {
		return nil, err
	}
	journalUpdate, err := memory.NewUpdateTaskTool(journal)
	if err != nil {
		return nil, err
	}
	analysis, err := toolsio.NewAnalysisTool()
	if err != nil {
		return nil, err
	}
	glossaryRegister, err := memory.NewRegisterTool(glossary)
	if err != nil {
		return nil, err
	}
	glossaryLookup, err := memory.NewLookupTool(glossary)
	if err != nil {
		return nil, err
	}
	critique, err := process.NewCritiqueTool()
	if err != nil {
		return nil, err
	}
	search, err := toolsio.NewSearchTool(b)
	if err != nil {
		return nil, err
	}
	merge, err := process.NewMergeTool()
	if err != nil {
		return nil, err
	}
	tables, err := toolsio.NewTableTool()
	if err != nil {
		return nil, err
	}
	repair, err := toolsio.NewRepairTool()
	if err != nil {
		return nil, err
	}
	extraction, err := toolsio.NewExtractionTool()
	if err != nil {
		return nil, err
	}
	briefing, err := process.NewBriefTool(brief)
	if err != nil {
		return nil, err
	}
	summary, err := process.NewSummaryTool()
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
2. Planning: Use 'journal_add_task' and 'analyze_dependencies' to map out the extraction.
3. Intelligence: Generate a 'generate_brief' to understand the core themes. This is your master blueprint.
4. Intelligent Execution (Context-Aware):
   - The Working Memory section above contains your current glossary, task list, and document brief — use it.
   - When calling 'goal_driven_synthesis', refer to the brief and glossary already in Working Memory.
   - If the current section references past topics, use 'semantic_search' to refresh your memory.
   - If you encounter a table, use 'extract_table_data' to preserve its logic.
5. Content Refinement: Use 'generate_html_summary' for each key item.
6. Quality Control:
   - Use 'quality_critique' to audit your work against the original source.
   - Use 'deduplicate_and_merge' to resolve redundant concepts across chapters.
7. Finalization: 'persist_knowledge_tree' only when the tree is architecturally sound.

You are self-correcting. Register new domain terms with 'glossary_register' as you encounter them.
Mark tasks complete with 'journal_update_task' as you finish them.`,
		Tools: []tool.Tool{
			chunking, synthesis, persistence,
			journalAdd, journalUpdate,
			analysis,
			glossaryRegister, glossaryLookup,
			critique, search, merge, tables, repair, extraction, briefing, summary,
		},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				workingMemory := b.RenderWorkingMemory()
				if req.Config == nil {
					req.Config = &genai.GenerateContentConfig{}
				}
				if req.Config.SystemInstruction == nil {
					req.Config.SystemInstruction = genai.NewContentFromText(workingMemory, "system")
				} else {
					existing := ""
					for _, part := range req.Config.SystemInstruction.Parts {
						existing += part.Text
					}
					req.Config.SystemInstruction = genai.NewContentFromText(existing+"\n\n"+workingMemory, "system")
				}
				return nil, nil
			},
		},
		AfterToolCallbacks: []llmagent.AfterToolCallback{
			func(ctx tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
				start := time.Now()
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

func (o *Orchestrator) ProcessDocument(ctx context.Context, runner *runner.Runner, jobID, documentID, workspaceID, fileURI, filename, mimeType string) error {
	if runner == nil {
		return fmt.Errorf("runner is not configured")
	}
	msg := fmt.Sprintf(
		"Process this document and build a knowledge tree.\n\njob_id: %s\ndocument_id: %s\nworkspace_id: %s\nfile_uri: %s\nfilename: %s\nmime_type: %s\n\nFollow your workflow: extract text, chunk, generate brief, synthesize items, critique, then persist.",
		jobID, documentID, workspaceID, fileURI, filename, mimeType,
	)
	for event, err := range runner.Run(ctx, "worker", jobID, genai.NewContentFromText(msg, genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("agent run: %w", err)
		}
		if event != nil && event.IsFinalResponse() {
			return nil
		}
	}
	return nil
}
