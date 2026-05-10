package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	toolsio "github.com/synthify/backend/apps/worker/pkg/worker/tools/io"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/memory"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/process"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/storage"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

type Orchestrator struct {
	Agent        agent.Agent
	currentJobID atomic.Pointer[string]
	base         *base.Context
	repo         repository.CheckpointRepository
	fs           *storage.FileSystem
}

// ToolLogger matches the repository interface for logging tool calls.
type ToolLogger interface {
	LogToolCall(ctx context.Context, jobID, toolName, inputJSON, outputJSON string, durationMs int64) error
}

var stageTools = map[string]string{
	"generate_brief":         "briefing",
	"goal_driven_synthesis": "synthesis",
	"persist_knowledge_tree": "persistence",
}

const currentCheckpointVersion = 1

func NewOrchestrator(m model.LLM, b *base.Context, repo any, fs *storage.FileSystem) (*Orchestrator, error) {
	checkpointRepo, _ := repo.(repository.CheckpointRepository)
	orch := &Orchestrator{
		base: b,
		repo: checkpointRepo,
		fs:   fs,
	}

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
	critique, err := process.NewCritiqueTool(b)
	if err != nil {
		return nil, err
	}
	search, err := toolsio.NewSearchTool(b)
	if err != nil {
		return nil, err
	}
	merge, err := process.NewMergeTool(b)
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
	extraction, err := toolsio.NewExtractionTool(b)
	if err != nil {
		return nil, err
	}
	briefing, err := process.NewBriefTool(b, brief)
	if err != nil {
		return nil, err
	}
	summary, err := process.NewSummaryTool(b)
	if err != nil {
		return nil, err
	}
	grep, err := toolsio.NewGrepTool(b)
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
			grep,
		},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				if err := b.IncrementLLMCalls(ctx); err != nil {
					return nil, err
				}
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
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{
			func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
				if err := b.IncrementToolRuns(ctx); err != nil {
					return nil, err
				}

				stage := stageTools[t.Name()]
				if stage == "" || orch.repo == nil || orch.fs == nil {
					return nil, nil
				}

				jobIDPtr := orch.currentJobID.Load()
				if jobIDPtr == nil || *jobIDPtr == "" {
					return nil, nil
				}
				jobID := *jobIDPtr

				// Check for existing checkpoint
				var envelope domain.CheckpointEnvelope
				found, err := orch.fs.ReadCheckpoint(jobID, stage, &envelope)
				if err != nil || !found {
					_ = orch.repo.UpsertStageRunning(ctx, jobID, stage)
					return nil, nil
				}

				// Validate checkpoint
				if envelope.SchemaVersion != currentCheckpointVersion {
					log.Printf("orchestrator: checkpoint version mismatch for stage %s: %d != %d", stage, envelope.SchemaVersion, currentCheckpointVersion)
					_ = orch.repo.UpsertStageRunning(ctx, jobID, stage)
					return nil, nil
				}

				// Basic input validation - compare document_id
				if b.Job != nil && envelope.DocumentID != b.Job.DocumentID {
					log.Printf("orchestrator: checkpoint document_id mismatch for stage %s", stage)
					_ = orch.repo.UpsertStageRunning(ctx, jobID, stage)
					return nil, nil
				}

				log.Printf("orchestrator: resuming stage %s from checkpoint", stage)
				return envelope.Outputs, nil
			},
		},
		AfterToolCallbacks: []llmagent.AfterToolCallback{
			func(ctx tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
				start := time.Now()
				argJSON, _ := json.Marshal(args)
				resJSON, _ := json.Marshal(result)
				if err != nil {
					resJSON, _ = json.Marshal(map[string]string{"error": err.Error()})
				}
				
				jobIDPtr := orch.currentJobID.Load()
				jobID := ""
				if jobIDPtr != nil {
					jobID = *jobIDPtr
				}

				if logger, ok := repo.(ToolLogger); ok && jobID != "" {
					_ = logger.LogToolCall(context.Background(), jobID, t.Name(), string(argJSON), string(resJSON), time.Since(start).Milliseconds())
				}

				// Save checkpoint if successful and stage-able
				stage := stageTools[t.Name()]
				if err == nil && stage != "" && jobID != "" && orch.repo != nil && orch.fs != nil {
					docID := ""
					wsID := ""
					if b.Job != nil {
						docID = b.Job.DocumentID
						wsID = b.Job.WorkspaceID
					}
					envelope := domain.CheckpointEnvelope{
						SchemaVersion: currentCheckpointVersion,
						Kind:          "synthify.worker_checkpoint",
						Stage:         stage,
						JobID:         jobID,
						DocumentID:    docID,
						WorkspaceID:   wsID,
						CreatedAt:     time.Now().UTC().Format(time.RFC3339),
						Inputs:        args,
						Outputs:       result,
					}
					if writeErr := orch.fs.WriteCheckpoint(jobID, stage, envelope); writeErr == nil {
						_ = orch.repo.MarkStageSucceeded(ctx, jobID, stage, orch.fs.CheckpointPath(jobID, stage))
					} else {
						log.Printf("orchestrator: failed to write checkpoint for stage %s: %v", stage, writeErr)
					}
				} else if err != nil && stage != "" && jobID != "" && orch.repo != nil {
					_ = orch.repo.MarkStageFailed(ctx, jobID, stage, err.Error())
				}

				return result, err
			},
		},
	})
	if err != nil {
		return nil, err
	}

	orch.Agent = a
	return orch, nil
}

func (o *Orchestrator) ProcessDocument(ctx context.Context, runner *runner.Runner, jobID, documentID, workspaceID, fileURI, filename, mimeType string) error {
	if runner == nil {
		return fmt.Errorf("runner is not configured")
	}
	if o.base != nil {
		o.base.BeginJob(ctx, jobID, workspaceID, documentID)
	}
	o.currentJobID.Store(&jobID)
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
