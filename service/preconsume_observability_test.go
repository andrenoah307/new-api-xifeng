package service

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureWarnLog redirects the warn/error writer used by logger.LogWarn so a
// test can assert on the exact backend log line.
func captureWarnLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	common.LogWriterMu.Lock()
	old := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = buf
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = old
		common.LogWriterMu.Unlock()
	})
	return buf
}

// One rejection must produce exactly one line: the mark is consumed on read so
// a later boundary cannot re-log the same attribution.
func TestPreConsumeRejectMarkIsConsumedOnce(t *testing.T) {
	c := periodBillingContext()
	markPreConsumeReject(c, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonWalletExhausted})

	details, ok := takePreConsumeRejectMark(c)
	require.True(t, ok)
	assert.Equal(t, PreConsumeRejectReasonWalletExhausted, details.Reason)

	_, ok = takePreConsumeRejectMark(c)
	assert.False(t, ok)

	_, ok = takePreConsumeRejectMark(nil)
	assert.False(t, ok)

	// Paths without a gin context (streaming reserve) must not panic on mark.
	markPreConsumeReject(nil, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonWalletReserve})
}

// wallet_first falls back to a subscription after a wallet rejection. That
// intermediate attribution must never be logged when the fallback succeeds,
// and must be replaced (not appended to) when the fallback also fails.
func TestPreConsumeRejectBoundaryOnlyLogsFinalAttribution(t *testing.T) {
	info := &relaycommon.RelayInfo{UserId: 42, TokenId: 7, OriginModelName: "test-model"}

	buf := captureWarnLog(t)
	c := periodBillingContext()
	markPreConsumeReject(c, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonWalletExhausted})
	logPreConsumeRejectBoundary(c, info, nil)
	assert.Empty(t, buf.String(), "a successful fallback never reaches the boundary with an error")

	markPreConsumeReject(c, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonSubscriptionQuota, BillingSource: BillingSourceSubscription})
	apiErr := types.NewError(fmt.Errorf("boom"), types.ErrorCodeInsufficientUserQuota)
	logPreConsumeRejectBoundary(c, info, apiErr)
	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "preconsume_reject "), "one rejection logs exactly one line")
	assert.Contains(t, out, "reason=subscription_quota_insufficient")
	assert.NotContains(t, out, "wallet_exhausted")
	assert.Contains(t, out, "error_code=insufficient_user_quota")

	// An error without any construction-point attribution (parameter validation
	// and the like) must stay silent rather than log a blank reason.
	buf.Reset()
	logPreConsumeRejectBoundary(c, info, apiErr)
	assert.Empty(t, buf.String())
}

// The period gate rejects outside BillingSession and therefore leaves no mark;
// the error code alone must still produce the correct attribution.
func TestPreConsumeRejectBoundaryAttributesPeriodGateByErrorCode(t *testing.T) {
	buf := captureWarnLog(t)
	c := periodBillingContext()
	c.Set("token_quota", 999)
	info := &relaycommon.RelayInfo{UserId: 42, TokenId: 7, OriginModelName: "test-model", UserQuota: 123}
	apiErr := types.NewError(fmt.Errorf("period exhausted"), types.ErrorCodeTokenPeriodQuotaExceeded)

	logPreConsumeRejectBoundary(c, info, apiErr)
	out := buf.String()
	assert.Contains(t, out, "reason=token_period_limit")
	assert.Contains(t, out, "error_code=token_period_quota_exceeded")
	assert.Contains(t, out, "user_quota=123")
	assert.Contains(t, out, "token_quota=999")
}

// The whole point of B1: the backend line must carry who was rejected, while
// the client-visible message stays byte-identical to the pre-B1 wording.
func TestPreConsumeBillingLogsWalletRejectionWithoutLeakingSecrets(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910031, 0, 0)
	buf := captureWarnLog(t)

	c := periodBillingContext()
	info := &relaycommon.RelayInfo{
		UserId: 910031, TokenId: 4242, TokenKey: "totally-secret-key",
		TokenUnlimited: true, IsPlayground: true, OriginModelName: "gpt-test",
	}
	apiErr := PreConsumeBilling(c, 8, 2, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, "用户额度不足, 剩余额度: "+logger.FormatQuota(0), apiErr.Error(), "client-visible wording must not change")

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "preconsume_reject "))
	assert.Contains(t, out, "reason=wallet_exhausted")
	assert.Contains(t, out, "billing_source=wallet")
	assert.Contains(t, out, "user_id=910031")
	assert.Contains(t, out, "token_id=4242")
	assert.Contains(t, out, `model="gpt-test"`)
	assert.NotContains(t, out, "totally-secret-key", "the token key must never reach the log")
}

// The estimate gate is a different rejection than an exhausted wallet: the user
// has money but cannot cover the input-only floor.
func TestPreConsumeBillingLogsEstimateGateSeparately(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910032, 3, 3)
	buf := captureWarnLog(t)

	c := periodBillingContext()
	info := &relaycommon.RelayInfo{
		UserId: 910032, TokenId: 4243, TokenUnlimited: true, IsPlayground: true,
		ForcePreConsume: true, OriginModelName: "gpt-test",
	}
	apiErr := PreConsumeBilling(c, 200, 150, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, buildInsufficientQuotaMessage(info, 3, 150, false), apiErr.Error())

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "preconsume_reject "))
	assert.Contains(t, out, "reason=wallet_estimate_gate")
	assert.Contains(t, out, "user_id=910032")
	assert.Contains(t, out, "full_quota=200")
	assert.Contains(t, out, "min_quota=150")
	assert.Contains(t, out, "user_quota=3")
}

// A successful pre-consume must stay silent — the log is for rejections only.
func TestPreConsumeBillingSuccessLogsNoRejection(t *testing.T) {
	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910033, common.GetTrustQuota()*3, common.GetTrustQuota()*3)
	buf := captureWarnLog(t)

	c := periodBillingContext()
	info := &relaycommon.RelayInfo{
		UserId: 910033, TokenId: 4244, TokenUnlimited: true, IsPlayground: true,
		OriginModelName: "gpt-test",
	}
	require.Nil(t, PreConsumeBilling(c, 8, 2, info))
	assert.NotContains(t, buf.String(), "preconsume_reject")
}

// reserveFunding runs on the streaming top-up path where there is no gin
// context and no PreConsumeBilling boundary, so it logs directly. A nil
// context must degrade to the SYSTEM request id instead of panicking.
func TestLogPreConsumeRejectWithoutGinContext(t *testing.T) {
	buf := captureWarnLog(t)
	info := &relaycommon.RelayInfo{UserId: 51, TokenId: 9, OriginModelName: "m", RequestId: "req-abc"}
	LogPreConsumeReject(nil, info, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonWalletReserve})

	out := buf.String()
	assert.Contains(t, out, "reason=wallet_reserve_miss")
	assert.Contains(t, out, `request_id="req-abc"`)
	assert.Contains(t, out, "SYSTEM")

	buf.Reset()
	LogPreConsumeReject(nil, nil, PreConsumeRejectDetails{Reason: PreConsumeRejectReasonWalletReserve})
	assert.Empty(t, buf.String())
}

// The condition-predicate reserve losing the race is doc 103's required
// post-deploy observable, and it must not move the wallet.
func TestReserveFundingLogsWalletReserveMiss(t *testing.T) {
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })

	setupServiceQuotaRedis(t)
	seedBillingWalletQuota(t, 910034, 5, 5)
	buf := captureWarnLog(t)

	info := &relaycommon.RelayInfo{UserId: 910034, TokenId: 4245, OriginModelName: "gpt-test", RequestId: "req-reserve"}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: 910034}}
	err := session.reserveFunding(50)
	require.Error(t, err)

	out := buf.String()
	assert.Contains(t, out, "reason=wallet_reserve_miss")
	assert.Contains(t, out, "user_id=910034")
	assert.Contains(t, out, "full_quota=50")

	var user model.User
	require.NoError(t, model.DB.First(&user, 910034).Error)
	assert.Equal(t, 5, user.Quota, "a lost reserve race must not move the wallet")
}
