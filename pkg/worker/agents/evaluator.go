package agents

import (
	"context"

	"github.com/synthify/backend/worker/pkg/worker/tools"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
)

type Evaluator struct {
	agent agent.Agent
}

func NewEvaluator(m model.LLM, base *tools.BaseContext) (*Evaluator, error) {
	a, err := llmagent.New(llmagent.Config{
		Name: "evaluator",
		Model: m,
		Instruction: `You are the quality assurance engineer for Synthify.
Analyze the generated knowledge tree and score its quality.
Focus on:
- Coverage: Did it capture all key concepts?
- Accuracy: Is it grounded in the original document?
- Structure: Is the tree logically organized for human exploration?`,
	})
	if err != nil {
		return nil, err
	}

	return &Evaluator{agent: a}, nil
}
func (e *Evaluator) EvaluateTree(ctx context.Context, jobID, treeData string) (string, error) {
	// ADK Agent.Run returns an iterator of events.
	// For now, we provide a placeholder as full event processing requires a session.
	return "Evaluation pending (ADK transition in progress)", nil
}
