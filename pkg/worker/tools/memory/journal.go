package memory

import (
	"fmt"
	"strings"
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type Task struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // "pending", "in_progress", "completed"
	DependsOn   []string `json:"depends_on,omitempty"`
}

type Journal struct {
	mu    sync.RWMutex
	tasks []Task
}

func NewJournal() *Journal {
	return &Journal{}
}

func (j *Journal) RenderForPrompt() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if len(j.tasks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Tasks\n")
	for _, t := range j.tasks {
		switch t.Status {
		case "completed":
			sb.WriteString("- [x] ")
		case "in_progress":
			sb.WriteString("- [~] ")
		default:
			sb.WriteString("- [ ] ")
		}
		sb.WriteString(t.ID)
		sb.WriteString(": ")
		sb.WriteString(t.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}

type addArgs struct {
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"description=IDs of tasks that must be completed before this one"`
}

type addResult struct {
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

func NewAddTaskTool(j *Journal) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "journal_add_task",
		Description: "Adds a new task to the processing checklist. Use this during planning to map out what needs to be done.",
	}, func(ctx tool.Context, args addArgs) (addResult, error) {
		j.mu.Lock()
		defer j.mu.Unlock()
		id := fmt.Sprintf("task_%d", len(j.tasks)+1)
		j.tasks = append(j.tasks, Task{
			ID:          id,
			Description: args.Description,
			Status:      "pending",
			DependsOn:   args.DependsOn,
		})
		return addResult{TaskID: id, Message: "Task added: " + id}, nil
	})
}

type updateArgs struct {
	TaskID string `json:"task_id"`
	Status string `json:"status" jsonschema:"enum=pending,in_progress,completed"`
}

type updateResult struct {
	Message string `json:"message"`
}

func NewUpdateTaskTool(j *Journal) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "journal_update_task",
		Description: "Updates the status of an existing task in the checklist.",
	}, func(ctx tool.Context, args updateArgs) (updateResult, error) {
		j.mu.Lock()
		defer j.mu.Unlock()
		for i, t := range j.tasks {
			if t.ID == args.TaskID {
				j.tasks[i].Status = args.Status
				return updateResult{Message: "Task updated: " + args.TaskID}, nil
			}
		}
		return updateResult{Message: "Task not found: " + args.TaskID}, nil
	})
}
