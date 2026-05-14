// Package metering wires LLM token usage from the worker's LLM calls into the
// billing API's BillingService.RecordUsage RPC.
//
// Pipeline:
//
//	gemini.GeminiClient -> metering.LLMClient -> base.CountingLLMClient -> tools
//
// metering.LLMClient reads token counts returned by llm.Client calls and ships
// the resulting domain.UsageEvent through llm.UsageReporter (a Connect client
// authenticated with the internal service token).
package metering

import "context"

// Tag carries the billing dimensions required to attribute LLM usage to an
// account/workspace/job. It is set in Worker.Process once the job's workspace
// has been resolved.
type Tag struct {
	AccountID   string
	WorkspaceID string
	JobID       string
}

type tagCtxKey struct{}

// WithTag returns a context that carries the given billing tag.
func WithTag(ctx context.Context, tag Tag) context.Context {
	return context.WithValue(ctx, tagCtxKey{}, tag)
}

// TagFromContext returns the billing tag stored on ctx, if any.
// The second return value indicates whether a tag was present.
func TagFromContext(ctx context.Context) (Tag, bool) {
	t, ok := ctx.Value(tagCtxKey{}).(Tag)
	if !ok {
		return Tag{}, false
	}
	return t, t.AccountID != ""
}
