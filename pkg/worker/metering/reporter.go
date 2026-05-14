package metering

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
)

// NewConnectReporter returns an llm.UsageReporter that ships usage events to
// the billing API over the Connect protocol. The internal service token is
// attached via the X-Synthify-Service-Token header on every request; the
// receiving handler requires it (middleware.IsServiceCall) before processing.
//
// apiBaseURL is the same URL the worker uses for other API RPCs (e.g.
// http://127.0.0.1:8080 in dev). serviceToken must match the API's
// SYNTHIFY_INTERNAL_SERVICE_TOKEN env var.
//
// When serviceToken is empty the returned reporter is a no-op; usage will be
// dropped silently with a single warn log at construction. This keeps local
// dev (where the token may not be configured) from breaking the worker.
func NewConnectReporter(apiBaseURL, serviceToken string) llm.UsageReporter {
	if apiBaseURL == "" || serviceToken == "" {
		return noopReporter{}
	}
	client := treev1connect.NewBillingServiceClient(http.DefaultClient, apiBaseURL)
	return &connectReporter{client: client, token: serviceToken}
}

type connectReporter struct {
	client treev1connect.BillingServiceClient
	token  string
}

func (r *connectReporter) RecordUsage(ctx context.Context, ev domain.UsageEvent) error {
	req := connect.NewRequest(&treev1.RecordUsageRequest{
		AccountId:    ev.AccountID,
		WorkspaceId:  ev.WorkspaceID,
		JobId:        ev.JobID,
		Model:        ev.Model,
		InputTokens:  ev.InputTokens,
		OutputTokens: ev.OutputTokens,
		EventId:      ev.EventID,
	})
	req.Header().Set("X-Synthify-Service-Token", r.token)
	_, err := r.client.RecordUsage(ctx, req)
	return err
}

type noopReporter struct{}

func (noopReporter) RecordUsage(context.Context, domain.UsageEvent) error { return nil }
