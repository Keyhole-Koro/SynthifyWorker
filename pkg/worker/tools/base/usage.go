package base

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/joblog"
)

type sessionIDContext interface {
	SessionID() string
}

type usageCounters struct {
	capability    *domain.JobCapability
	llmCalls      int
	toolRuns      int
	itemCreations int
}

// UsageLimiter tracks per-job worker resource usage and enforces JobCapability limits.
type UsageLimiter struct {
	repo Repository

	mu   sync.Mutex
	jobs map[string]*usageCounters
}

func NewUsageLimiter(repo Repository) *UsageLimiter {
	return &UsageLimiter{
		repo: repo,
		jobs: make(map[string]*usageCounters),
	}
}

func (l *UsageLimiter) BeginJob(ctx context.Context, jobID string) {
	if l == nil || jobID == "" {
		return
	}
	capability, _ := l.loadCapability(ctx, jobID)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.jobs[jobID] = &usageCounters{capability: capability}
}

func (l *UsageLimiter) IncrementLLMCalls(ctx context.Context) error {
	return l.increment(ctx, 1, 0, 0)
}

func (l *UsageLimiter) IncrementToolRuns(ctx context.Context) error {
	return l.increment(ctx, 0, 1, 0)
}

func (l *UsageLimiter) IncrementItemCreations(ctx context.Context, count int) error {
	if count <= 0 {
		return nil
	}
	return l.increment(ctx, 0, 0, count)
}

func (l *UsageLimiter) increment(ctx context.Context, llmCalls, toolRuns, itemCreations int) error {
	if l == nil {
		return nil
	}
	jobID := jobIDFromContext(ctx)
	if jobID == "" {
		return nil
	}

	l.mu.Lock()
	counters := l.jobs[jobID]
	l.mu.Unlock()

	if counters == nil {
		capability, _ := l.loadCapability(ctx, jobID)
		l.mu.Lock()
		counters = l.jobs[jobID]
		if counters == nil {
			counters = &usageCounters{capability: capability}
			l.jobs[jobID] = counters
		}
		l.mu.Unlock()
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	counters.llmCalls += llmCalls
	counters.toolRuns += toolRuns
	counters.itemCreations += itemCreations
	capability := counters.capability
	if capability == nil {
		return nil
	}
	if capability.MaxLLMCalls > 0 && counters.llmCalls > capability.MaxLLMCalls {
		log.Printf("usage limit exceeded: job=%s llm_calls=%d/%d", jobID, counters.llmCalls, capability.MaxLLMCalls)
		joblog.FromContext(ctx).Log(ctx, joblog.Event{
			JobID:   jobID,
			Level:   joblog.ERROR,
			Event:   "llm.usage_exceeded",
			Message: fmt.Sprintf("LLM call limit exceeded: %d/%d", counters.llmCalls, capability.MaxLLMCalls),
			Detail:  map[string]any{"resource": "llm_calls", "count": counters.llmCalls, "limit": capability.MaxLLMCalls},
		})
		return fmt.Errorf("job %s exceeded LLM call limit: %d > %d", jobID, counters.llmCalls, capability.MaxLLMCalls)
	}
	if capability.MaxToolRuns > 0 && counters.toolRuns > capability.MaxToolRuns {
		log.Printf("usage limit exceeded: job=%s tool_runs=%d/%d", jobID, counters.toolRuns, capability.MaxToolRuns)
		joblog.FromContext(ctx).Log(ctx, joblog.Event{
			JobID:   jobID,
			Level:   joblog.ERROR,
			Event:   "llm.usage_exceeded",
			Message: fmt.Sprintf("tool run limit exceeded: %d/%d", counters.toolRuns, capability.MaxToolRuns),
			Detail:  map[string]any{"resource": "tool_runs", "count": counters.toolRuns, "limit": capability.MaxToolRuns},
		})
		return fmt.Errorf("job %s exceeded tool run limit: %d > %d", jobID, counters.toolRuns, capability.MaxToolRuns)
	}
	if capability.MaxItemCreations > 0 && counters.itemCreations > capability.MaxItemCreations {
		log.Printf("usage limit exceeded: job=%s item_creations=%d/%d", jobID, counters.itemCreations, capability.MaxItemCreations)
		joblog.FromContext(ctx).Log(ctx, joblog.Event{
			JobID:   jobID,
			Level:   joblog.ERROR,
			Event:   "llm.usage_exceeded",
			Message: fmt.Sprintf("item creation limit exceeded: %d/%d", counters.itemCreations, capability.MaxItemCreations),
			Detail:  map[string]any{"resource": "item_creations", "count": counters.itemCreations, "limit": capability.MaxItemCreations},
		})
		return fmt.Errorf("job %s exceeded item creation limit: %d > %d", jobID, counters.itemCreations, capability.MaxItemCreations)
	}
	return nil
}

func (l *UsageLimiter) loadCapability(ctx context.Context, jobID string) (*domain.JobCapability, error) {
	if l == nil || l.repo == nil || jobID == "" {
		return nil, domain.ErrNotFound
	}
	return l.repo.GetJobCapability(ctx, jobID)
}

func jobIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if sessionCtx, ok := ctx.(sessionIDContext); ok {
		return sessionCtx.SessionID()
	}
	return ""
}

// CountingLLMClient counts direct tool-level LLM calls before delegating to the real client.
type CountingLLMClient struct {
	inner LLMClient
	usage *UsageLimiter
}

func NewCountingLLMClient(inner LLMClient, usage *UsageLimiter) LLMClient {
	if inner == nil {
		return nil
	}
	return &CountingLLMClient{inner: inner, usage: usage}
}

func (c *CountingLLMClient) GenerateStructured(ctx context.Context, req llm.StructuredRequest) (json.RawMessage, error) {
	if c != nil && c.usage != nil {
		if err := c.usage.IncrementLLMCalls(ctx); err != nil {
			return nil, err
		}
	}
	return c.inner.GenerateStructured(ctx, req)
}

func (c *CountingLLMClient) GenerateText(ctx context.Context, req llm.TextRequest) (string, error) {
	if c != nil && c.usage != nil {
		if err := c.usage.IncrementLLMCalls(ctx); err != nil {
			return "", err
		}
	}
	return c.inner.GenerateText(ctx, req)
}
