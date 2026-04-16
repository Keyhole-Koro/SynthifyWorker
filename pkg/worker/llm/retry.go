package llm

import (
	"context"
	"encoding/json"
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

func (c *RetryingClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		out, err := c.inner.GenerateStructured(ctx, req)
		if err == nil && json.Valid(out) {
			return out, nil
		}
		if err == nil {
			repaired := RepairJSON(string(out))
			if json.Valid([]byte(repaired)) {
				return json.RawMessage(repaired), nil
			}
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *RetryingClient) GenerateText(ctx context.Context, req TextRequest) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		out, err := c.inner.GenerateText(ctx, req)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", lastErr
}
