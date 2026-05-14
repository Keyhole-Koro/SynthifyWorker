package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

type RetryingClient struct {
	inner      Client
	maxRetries int
}

func NewRetryingClient(inner Client, maxRetries int) *RetryingClient {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &RetryingClient{inner: inner, maxRetries: maxRetries}
}

func (c *RetryingClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error) {
	var lastErr error
	var lastUsage Usage
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		out, usage, err := c.inner.GenerateStructured(ctx, req)
		lastUsage = usage
		if err == nil && json.Valid(out) {
			return out, usage, nil
		}
		if err == nil {
			repaired := RepairJSON(string(out))
			if json.Valid([]byte(repaired)) {
				return json.RawMessage(repaired), usage, nil
			}
			err = fmt.Errorf("invalid JSON response")
		}
		lastErr = err
	}
	return nil, lastUsage, lastErr
}

func (c *RetryingClient) GenerateText(ctx context.Context, req TextRequest) (string, Usage, error) {
	var lastErr error
	var lastUsage Usage
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		out, usage, err := c.inner.GenerateText(ctx, req)
		lastUsage = usage
		if err == nil {
			return out, usage, nil
		}
		lastErr = err
	}
	return "", lastUsage, lastErr
}
