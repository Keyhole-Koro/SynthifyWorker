package tools

import (
	"fmt"
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Task struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // "pending", "in_progress", "completed"
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"description=IDs of tasks that must be completed before this one"`
}

// MemoryJournal is a temporary in-memory store for the checklist.
// In production, this would be backed by Postgres.
type MemoryJournal struct {
	mu    sync.RWMutex
	tasks map[string][]Task
}

var globalJournal = &MemoryJournal{tasks: make(map[string][]Task)}

type JournalArgs struct {
	JobID       string   `json:"job_id"`
	Action      string   `json:"action" jsonschema:"enum=add,update,list,get_next_pending,description=The action to perform on the checklist"`
	TaskID      string   `json:"task_id,omitempty"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type JournalResult struct {
	Tasks   []Task `json:"tasks,omitempty"`
	Message string `json:"message"`
}

// NewJournalTool manages an in-memory per-job checklist used by the orchestrator.
// Input schema: JournalArgs{job_id: string, action: string, task_id?: string, description?: string, status?: string, depends_on?: []string}.
// Output schema: JournalResult{tasks?: []Task, message: string}.
func NewJournalTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "manage_job_checklist",
		Description: "Manages a persistent checklist of tasks. Tasks can have dependencies (depends_on). Use this to plan the processing order dynamically.",
	}, func(ctx tool.Context, args JournalArgs) (JournalResult, error) {
		globalJournal.mu.Lock()
		defer globalJournal.mu.Unlock()

		jobTasks := globalJournal.tasks[args.JobID]

		switch args.Action {
		case "add":
			newTask := Task{
				ID:          fmt.Sprintf("task_%d", len(jobTasks)+1),
				Description: args.Description,
				Status:      "pending",
				DependsOn:   args.DependsOn,
			}
			globalJournal.tasks[args.JobID] = append(jobTasks, newTask)
			return JournalResult{Message: "Task added: " + newTask.ID}, nil

		case "update":
			for i, t := range jobTasks {
				if t.ID == args.TaskID {
					if args.Status != "" {
						jobTasks[i].Status = args.Status
					}
					globalJournal.tasks[args.JobID] = jobTasks
					return JournalResult{Message: "Task updated: " + args.TaskID}, nil
				}
			}
			return JournalResult{Message: "Task not found"}, nil

		case "get_next_pending":
			for _, t := range jobTasks {
				if t.Status == "pending" {
					// Check if all dependencies are completed
					depsMet := true
					for _, depID := range t.DependsOn {
						foundComp := false
						for _, dt := range jobTasks {
							if dt.ID == depID && dt.Status == "completed" {
								foundComp = true
								break
							}
						}
						if !foundComp {
							depsMet = false
							break
						}
					}
					if depsMet {
						return JournalResult{Tasks: []Task{t}, Message: "Next available task"}, nil
					}
				}
			}
			return JournalResult{Message: "No available tasks (waiting for dependencies or finished)"}, nil

		case "list":
			return JournalResult{Tasks: jobTasks, Message: "Retrieved tasks"}, nil

		default:
			return JournalResult{Message: "Invalid action"}, nil
		}
	})
}
