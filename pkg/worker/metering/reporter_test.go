package metering

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
)

func TestConnectReporter_RecordUsage_ForwardsUsageEventFields(t *testing.T) {
	// 目的: worker 側で生成した UsageEvent が Connect request に欠落なく詰め替えられることを確認する。
	// event_id は API/DB 側の冪等性キーなので、ここで落とすと実経路だけが失敗する。
	client := &billingClientStub{}
	reporter := &connectReporter{client: client, token: "service-token"}

	err := reporter.RecordUsage(context.Background(), domain.UsageEvent{
		EventID:      "evt-worker-1",
		AccountID:    "acc-1",
		WorkspaceID:  "ws-1",
		JobID:        "job-1",
		Model:        "gemini-3-flash-preview",
		InputTokens:  123,
		OutputTokens: 45,
	})

	require.NoError(t, err)
	require.NotNil(t, client.recordUsageReq)
	assert.Equal(t, "service-token", client.recordUsageReq.Header().Get("X-Synthify-Service-Token"))
	assert.Equal(t, "evt-worker-1", client.recordUsageReq.Msg.GetEventId())
	assert.Equal(t, "acc-1", client.recordUsageReq.Msg.GetAccountId())
	assert.Equal(t, "ws-1", client.recordUsageReq.Msg.GetWorkspaceId())
	assert.Equal(t, "job-1", client.recordUsageReq.Msg.GetJobId())
	assert.Equal(t, "gemini-3-flash-preview", client.recordUsageReq.Msg.GetModel())
	assert.Equal(t, int64(123), client.recordUsageReq.Msg.GetInputTokens())
	assert.Equal(t, int64(45), client.recordUsageReq.Msg.GetOutputTokens())
}

type billingClientStub struct {
	recordUsageReq *connect.Request[treev1.RecordUsageRequest]
}

func (c *billingClientStub) GetBillingAccount(context.Context, *connect.Request[treev1.GetBillingAccountRequest]) (*connect.Response[treev1.GetBillingAccountResponse], error) {
	return connect.NewResponse(&treev1.GetBillingAccountResponse{}), nil
}

func (c *billingClientStub) CreateCheckoutSession(context.Context, *connect.Request[treev1.CreateCheckoutSessionRequest]) (*connect.Response[treev1.CreateCheckoutSessionResponse], error) {
	return connect.NewResponse(&treev1.CreateCheckoutSessionResponse{}), nil
}

func (c *billingClientStub) CreatePortalSession(context.Context, *connect.Request[treev1.CreatePortalSessionRequest]) (*connect.Response[treev1.CreatePortalSessionResponse], error) {
	return connect.NewResponse(&treev1.CreatePortalSessionResponse{}), nil
}

func (c *billingClientStub) GetUsage(context.Context, *connect.Request[treev1.GetUsageRequest]) (*connect.Response[treev1.GetUsageResponse], error) {
	return connect.NewResponse(&treev1.GetUsageResponse{}), nil
}

func (c *billingClientStub) RecordUsage(ctx context.Context, req *connect.Request[treev1.RecordUsageRequest]) (*connect.Response[treev1.RecordUsageResponse], error) {
	c.recordUsageReq = req
	return connect.NewResponse(&treev1.RecordUsageResponse{}), nil
}

func (c *billingClientStub) UpdateBudget(context.Context, *connect.Request[treev1.UpdateBudgetRequest]) (*connect.Response[treev1.UpdateBudgetResponse], error) {
	return connect.NewResponse(&treev1.UpdateBudgetResponse{}), nil
}

func (c *billingClientStub) ListInvoices(context.Context, *connect.Request[treev1.ListInvoicesRequest]) (*connect.Response[treev1.ListInvoicesResponse], error) {
	return connect.NewResponse(&treev1.ListInvoicesResponse{}), nil
}

func (c *billingClientStub) ListPaymentMethods(context.Context, *connect.Request[treev1.ListPaymentMethodsRequest]) (*connect.Response[treev1.ListPaymentMethodsResponse], error) {
	return connect.NewResponse(&treev1.ListPaymentMethodsResponse{}), nil
}
