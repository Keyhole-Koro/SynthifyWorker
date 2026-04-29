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

type MemoryJournal struct {
	mu    sync.RWMutex
	tasks []Task
}

func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{}
}

type JournalArgs struct {
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

func NewJournalTool(base *BaseContext) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "manage_job_checklist",
		Description: "Manages a persistent checklist of tasks. Tasks can have dependencies (depends_on). Use this to plan the processing order dynamically.",
	}, func(ctx tool.Context, args JournalArgs) (JournalResult, error) {
		j := base.Journal
		j.mu.Lock()
		defer j.mu.Unlock()

		switch args.Action {
		case "add":
			newTask := Task{
				ID:          fmt.Sprintf("task_%d", len(j.tasks)+1),
				Description: args.Description,
				Status:      "pending",
				DependsOn:   args.DependsOn,
			}
			j.tasks = append(j.tasks, newTask)
			return JournalResult{Message: "Task added: " + newTask.ID}, nil

		case "update":
			for i, t := range j.tasks {
				if t.ID == args.TaskID {
					if args.Status != "" {
						j.tasks[i].Status = args.Status
					}
					return JournalResult{Message: "Task updated: " + args.TaskID}, nil
				}
			}
			return JournalResult{Message: "Task not found"}, nil

		case "get_next_pending":
			for _, t := range j.tasks {
				if t.Status != "pending" {
					continue
				}
				depsMet := true
				for _, depID := range t.DependsOn {
					foundComp := false
					for _, dt := range j.tasks {
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
			return JournalResult{Message: "No available tasks (waiting for dependencies or finished)"}, nil

		case "list":
			return JournalResult{Tasks: j.tasks, Message: "Retrieved tasks"}, nil

		default:
			return JournalResult{Message: "Invalid action"}, nil
		}
	})
}
