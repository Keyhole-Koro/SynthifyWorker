package stages

import (
	"context"
	"encoding/json"

	workerllm "github.com/synthify/backend/worker/pkg/worker/llm"
)

type fakeLLMClient struct {
	structured json.RawMessage
	text       string
	lastStruct workerllm.StructuredRequest
	lastText   workerllm.TextRequest
}

func (f *fakeLLMClient) GenerateStructured(ctx context.Context, req workerllm.StructuredRequest) (json.RawMessage, error) {
	f.lastStruct = req
	return append(json.RawMessage(nil), f.structured...), nil
}

func (f *fakeLLMClient) GenerateText(ctx context.Context, req workerllm.TextRequest) (string, error) {
	f.lastText = req
	return f.text, nil
}
