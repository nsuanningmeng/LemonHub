package relay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deniedTaskReservation struct{ target int }

func (*deniedTaskReservation) Settle(int) error         { return nil }
func (*deniedTaskReservation) Refund(*gin.Context)      {}
func (*deniedTaskReservation) NeedsRefund() bool        { return true }
func (*deniedTaskReservation) GetPreConsumedQuota() int { return 1 }
func (*deniedTaskReservation) Reserve(int) error {
	return errors.New("post-submission reservation must not run")
}
func (s *deniedTaskReservation) ReserveBeforeSubmit(target int) error {
	s.target = target
	return types.NewErrorWithStatusCode(errors.New("wallet quota insufficient"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
}

func TestNativeTaskRetryDoesNotContactUpstreamBeforeRequiredReservation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"sora-2","prompt":"a sunset","seconds":"4"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("platform", strconv.Itoa(constant.ChannelTypeSora))
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeSora)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	billing := &deniedTaskReservation{}
	info := &relaycommon.RelayInfo{OriginModelName: "sora-2", UsingGroup: "default", UserGroup: "default", RelayFormat: types.RelayFormatTask, TaskRelayInfo: &relaycommon.TaskRelayInfo{}, Billing: billing, TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: "stale previous price"}}
	result, taskErr := RelayTaskSubmit(c, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
	assert.Positive(t, billing.target)
	assert.Zero(t, calls.Load())
	assert.Nil(t, info.TieredBillingSnapshot)
}
