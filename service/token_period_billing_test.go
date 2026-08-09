package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func countTokenPeriodStateQueries(t *testing.T) *atomic.Int64 {
	t.Helper()
	var count atomic.Int64
	callbackName := "test:count_token_period_state:" + strings.ReplaceAll(t.Name(), "/", "_")
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.TokenPeriodState); ok {
			count.Add(1)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	})
	return &count
}

func periodBillingContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	return c
}

func seedBillingPeriodToken(t *testing.T, id, userID int, key string, used int64, limit int64) *model.Token {
	t.Helper()
	now := time.Now()
	anchor := common.TokenPeriodAnchorNow(now)
	start, _, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 1, anchor, now)
	require.True(t, ok)
	token := &model.Token{
		Id:               id,
		UserId:           userID,
		Key:              key,
		Status:           common.TokenStatusEnabled,
		RemainQuota:      100000000,
		PeriodType:       common.TokenPeriodTypeDays,
		PeriodDays:       1,
		PeriodQuotaLimit: limit,
		PeriodLimitUnit:  "quota",
		PeriodAnchorAt:   anchor,
		PeriodStartAt:    start,
		PeriodUsedQuota:  used,
	}
	require.NoError(t, model.DB.Create(token).Error)
	return token
}

func TestTokenPeriodExceededMessageUsesFixedCNYAndUTC8(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldRate := operationSettingUSDExchangeRate()
	common.QuotaPerUnit = 500000
	setOperationSettingUSDExchangeRate(7.3)
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		setOperationSettingUSDExchangeRate(oldRate)
	})

	now := time.Date(2026, 8, 2, 8, 0, 0, 0, time.UTC)
	state := &model.TokenPeriodState{
		Type:     common.TokenPeriodTypeDays,
		Days:     1,
		Limit:    684932,
		AnchorAt: common.TokenPeriodAnchorNow(now),
	}
	message := buildTokenPeriodQuotaExceededMessage(state, 701370, now)

	assert.Contains(t, message, "本周期已用 ¥10.24（701370 quota）")
	assert.Contains(t, message, "上限 ¥10.00（684932 quota）")
	assert.Contains(t, message, "下次重置时间 2026-08-03 00:00:00 UTC+8")
	assert.NotContains(t, message, "分组")
	assert.NotContains(t, message, "倍率")
	assert.NotContains(t, message, "渠道")
}

func TestBillingSessionPeriodGateIsSoftAndCapturesAttribution(t *testing.T) {
	truncate(t)
	const userID, tokenID = 701, 702
	seedUser(t, userID, common.GetTrustQuota()+100000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-soft", 90, 100)

	info := &relaycommon.RelayInfo{
		TokenId:        token.Id,
		TokenKey:       token.Key,
		TokenUnlimited: true,
		UserId:         userID,
		UserQuota:      common.GetTrustQuota() + 100000,
	}
	session := &BillingSession{
		relayInfo: info,
		funding:   &WalletFunding{userId: userID},
	}

	apiErr := session.preConsume(periodBillingContext(), 1000000)
	require.Nil(t, apiErr, "E3 allows a request when used < limit even if estimate exceeds remaining space")
	assert.Equal(t, token.PeriodStartAt, info.TokenPeriodStartAt)
	assert.Equal(t, 0, session.GetPreConsumedQuota(), "trusted requests retain the trust bypass")
}

func TestBillingSessionPeriodGateRejectsWithContractForTrustedAndUntrusted(t *testing.T) {
	truncate(t)
	const userID, tokenID = 703, 704
	seedUser(t, userID, common.GetTrustQuota()+100000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-full", 100, 100)

	for _, tc := range []struct {
		name         string
		unlimited    bool
		userQuota    int
		forceConsume bool
	}{
		{name: "trusted", unlimited: true, userQuota: common.GetTrustQuota() + 100000},
		{name: "untrusted", unlimited: false, userQuota: 1000, forceConsume: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				TokenId:         token.Id,
				TokenKey:        token.Key,
				TokenUnlimited:  tc.unlimited,
				UserId:          userID,
				UserQuota:       tc.userQuota,
				ForcePreConsume: tc.forceConsume,
			}
			session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}
			apiErr := session.preConsume(periodBillingContext(), 100)
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorCodeTokenPeriodQuotaExceeded, apiErr.GetErrorCode())
			assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.False(t, types.IsRecordErrorLog(apiErr))
			assert.NotContains(t, apiErr.Error(), "insufficient_user_quota")
			assert.True(t, strings.Contains(apiErr.Error(), "used") || strings.Contains(apiErr.Error(), "已用"))
			assert.Contains(t, apiErr.Error(), "上限")
			assert.Contains(t, apiErr.Error(), "重置")
		})
	}
}

func TestUnlimitedTokenPeriodGateDoesNotNeedTokenQuotaContextAndCounts(t *testing.T) {
	truncate(t)
	const userID, tokenID = 705, 706
	seedUser(t, userID, 10000000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-unlimited", 0, 1000)

	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000000,
		ForcePreConsume: true,
	}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}
	apiErr := session.preConsume(periodBillingContext(), 37)
	require.Nil(t, apiErr)

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(37), updated.PeriodUsedQuota)
	assert.Equal(t, info.TokenPeriodStartAt, updated.PeriodStartAt)
}

func TestPlaygroundTokenPeriodGateSkipsDatabase(t *testing.T) {
	oldDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = oldDB })

	info := &relaycommon.RelayInfo{IsPlayground: true, TokenId: 999999, TokenKey: ""}
	require.NoError(t, PreConsumeTokenQuota(info, 123))
	assert.Zero(t, info.TokenPeriodStartAt)
}

func TestTaskPrivateDataRoundTripsTokenPeriodAttribution(t *testing.T) {
	privateData := model.TaskPrivateData{TokenId: 42, TokenPeriodStartAt: 1785686400}
	value, err := privateData.Value()
	require.NoError(t, err)
	var decoded model.TaskPrivateData
	require.NoError(t, decoded.Scan(value))
	assert.Equal(t, privateData.TokenId, decoded.TokenId)
	assert.Equal(t, privateData.TokenPeriodStartAt, decoded.TokenPeriodStartAt)
}

func TestTokenPeriodRefundUsesOriginalAttribution(t *testing.T) {
	truncate(t)
	const userID, tokenID = 707, 708
	seedUser(t, userID, 100000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-refund", 0, 1000)
	start := token.PeriodStartAt
	require.NoError(t, model.AdjustTokenQuota(token.Id, token.Key, 80, start, nil))

	info := &relaycommon.RelayInfo{
		TokenId:            token.Id,
		TokenKey:           token.Key,
		TokenPeriodStartAt: start,
		UserId:             userID,
		UserQuota:          100000,
		ForcePreConsume:    true,
	}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}, preConsumedQuota: 80, tokenConsumed: 80, tokenPeriodStartAt: start}
	// A negative settlement is the same signed primitive as a refund.
	require.NoError(t, session.Settle(20))

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(20), updated.PeriodUsedQuota)
}

func TestWssSegmentsCountAndRefreshStaleBucket(t *testing.T) {
	truncate(t)
	const userID, tokenID = 709, 710
	seedUser(t, userID, 10000000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-wss", 0, 1000)
	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserGroup:       "default",
		UsingGroup:      "default",
		OriginModelName: "gpt-4o-realtime-preview",
	}
	usage := &dto.RealtimeUsage{
		TotalTokens:       10,
		InputTokens:       10,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 10},
	}
	queries := countTokenPeriodStateQueries(t)

	require.NoError(t, PreWssConsumeQuota(periodBillingContext(), info, usage))
	assert.Equal(t, int64(2), queries.Load(), "an enabled segment reads once for its gate and once for its atomic adjustment")
	var first model.Token
	require.NoError(t, model.DB.First(&first, token.Id).Error)
	require.Positive(t, first.PeriodUsedQuota)
	firstCharge := first.PeriodUsedQuota

	// Simulate the next segment arriving after a period boundary. A stale
	// persisted bucket is logically empty and the segment resets/counts into
	// the current bucket in one atomic adjustment.
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_start_at":   token.PeriodStartAt - 24*60*60,
		"period_used_quota": 999,
	}).Error)
	info.TokenPeriodStartAt = token.PeriodStartAt - 24*60*60
	require.NoError(t, PreWssConsumeQuota(periodBillingContext(), info, usage))
	assert.Equal(t, int64(4), queries.Load(), "each WSS segment refreshes its authoritative gate and enabled adjustment")

	var second model.Token
	require.NoError(t, model.DB.First(&second, token.Id).Error)
	assert.Equal(t, token.PeriodStartAt, second.PeriodStartAt)
	assert.Equal(t, firstCharge, second.PeriodUsedQuota)
	assert.Equal(t, token.PeriodStartAt, info.TokenPeriodStartAt)
}

func TestWssPeriodGateRejectsAnExhaustedUnlimitedToken(t *testing.T) {
	truncate(t)
	const userID, tokenID = 711, 712
	seedUser(t, userID, 10000000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-wss-full", 100, 100)
	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserGroup:       "default",
		UsingGroup:      "default",
		OriginModelName: "gpt-4o-realtime-preview",
	}
	usage := &dto.RealtimeUsage{
		TotalTokens:       1,
		InputTokens:       1,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 1},
	}

	err := PreWssConsumeQuota(periodBillingContext(), info, usage)
	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, types.ErrorCodeTokenPeriodQuotaExceeded, apiErr.GetErrorCode())
}

type failingPeriodFunding struct{}

func (failingPeriodFunding) Source() string       { return BillingSourceWallet }
func (failingPeriodFunding) PreConsume(int) error { return assert.AnError }
func (failingPeriodFunding) Settle(int) error     { return nil }
func (failingPeriodFunding) Refund() error        { return nil }

func TestBillingSessionReserveSettleAndPreConsumeRollbackClosePeriodDelta(t *testing.T) {
	truncate(t)
	const userID, tokenID = 713, 714
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-close", 0, 1000)
	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000,
		ForcePreConsume: true,
	}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}
	require.Nil(t, session.preConsume(periodBillingContext(), 100))
	require.NoError(t, session.Reserve(150))
	require.NoError(t, session.Settle(100))

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(100), updated.PeriodUsedQuota)
	assert.Equal(t, 9900, getUserQuota(t, userID))

	// A funding failure after token pre-consume must roll the same bucket back.
	second := seedBillingPeriodToken(t, tokenID+1, userID, "period-rollback", 0, 1000)
	rollbackInfo := &relaycommon.RelayInfo{
		TokenId:         second.Id,
		TokenKey:        second.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000,
		ForcePreConsume: true,
	}
	rollback := &BillingSession{relayInfo: rollbackInfo, funding: failingPeriodFunding{}}
	apiErr := rollback.preConsume(periodBillingContext(), 40)
	require.NotNil(t, apiErr)
	var rolledBack model.Token
	require.NoError(t, model.DB.First(&rolledBack, second.Id).Error)
	assert.Zero(t, rolledBack.PeriodUsedQuota)
}

func TestTaskFailureRefundUsesStoredPeriodAttribution(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID = 715, 716, 717
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-task", 80, 1000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, 50, tokenID, BillingSourceWallet, 0)
	task.PrivateData.TokenPeriodStartAt = token.PeriodStartAt

	RefundTaskQuota(context.Background(), task, "period task failed")

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(30), updated.PeriodUsedQuota)
	assert.Equal(t, 10000+50, getUserQuota(t, userID))

	// An old-bucket failure refunds the balance but cannot reduce a new bucket.
	second := seedBillingPeriodToken(t, tokenID+1, userID, "period-task-old", 0, 1000)
	oldTask := makeTask(userID, channelID, 25, second.Id, BillingSourceWallet, 0)
	oldTask.PrivateData.TokenPeriodStartAt = second.PeriodStartAt - 24*60*60
	RefundTaskQuota(context.Background(), oldTask, "cross period failure")
	var current model.Token
	require.NoError(t, model.DB.First(&current, second.Id).Error)
	assert.Zero(t, current.PeriodUsedQuota)
}

func TestMidjourneyFailureRefundsTokenOnceAndDoesNotGuessLegacyToken(t *testing.T) {
	truncate(t)
	const userID, tokenID = 718, 719
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-mj", 60, 1000)
	task := &model.Midjourney{
		UserId:             userID,
		MjId:               "mj-period-refund",
		TokenId:            token.Id,
		TokenPeriodStartAt: token.PeriodStartAt,
		Quota:              20,
	}

	require.NoError(t, RefundMidjourneyTokenQuota(context.Background(), task))
	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(40), updated.PeriodUsedQuota)

	// A historical row without TokenId is warning-only and cannot mutate this
	// token (or any guessed token belonging to the same user).
	historical := &model.Midjourney{UserId: userID, MjId: "mj-legacy", Quota: 20}
	require.NoError(t, RefundMidjourneyTokenQuota(context.Background(), historical))
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, int64(40), updated.PeriodUsedQuota)
}

func TestTokenPeriodHelperDefensiveBranches(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldRate
	})
	common.QuotaPerUnit = 0
	operation_setting.USDExchangeRate = 0
	assert.Equal(t, "¥0.00", tokenPeriodCNY(10))
	assert.Equal(t, "未知", tokenPeriodResetText(0))
	assert.Equal(t, "令牌周期限额已用尽", buildTokenPeriodQuotaExceededMessage(nil, 1, time.Now()))
	assert.Nil(t, CheckTokenPeriodGate(nil, 10))
	assert.Nil(t, CheckTokenPeriodGate(&relaycommon.RelayInfo{IsPlayground: true, TokenId: 1}, 10))
}

func TestTokenPeriodAttributionAndGateStateBranches(t *testing.T) {
	truncate(t)
	const userID, tokenID = 720, 721
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-helper", 0, 100)
	info := &relaycommon.RelayInfo{TokenId: token.Id}
	start, err := loadTokenPeriodAttribution(info, true)
	require.NoError(t, err)
	assert.Equal(t, token.PeriodStartAt, start)
	// Reuse avoids a second database read in the normal request lifecycle.
	reused, err := loadTokenPeriodAttribution(info, false)
	require.NoError(t, err)
	assert.Equal(t, start, reused)

	state, used, apiErr := checkTokenPeriodGate(info, time.Now())
	require.Nil(t, apiErr)
	assert.NotNil(t, state)
	assert.Zero(t, used)

	// Disabled policy is a no-op and clears a stale attribution.
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_type":        "",
		"period_quota_limit": 0,
	}).Error)
	info.TokenPeriodStartAt = start
	_, err = loadTokenPeriodAttribution(info, true)
	require.NoError(t, err)
	assert.Zero(t, info.TokenPeriodStartAt)
	_, used, apiErr = checkTokenPeriodGate(info, time.Now())
	require.Nil(t, apiErr)
	assert.Zero(t, used)

	// Invalid enabled policy fails closed rather than allowing a request.
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_type":        "invalid",
		"period_quota_limit": 1,
	}).Error)
	_, _, apiErr = checkTokenPeriodGate(&relaycommon.RelayInfo{TokenId: token.Id}, time.Now())
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeQueryDataError, apiErr.GetErrorCode())
	_, err = loadTokenPeriodAttribution(&relaycommon.RelayInfo{TokenId: token.Id}, true)
	assert.Error(t, err)

	// Missing rows are query errors, not an allow decision.
	_, _, apiErr = checkTokenPeriodGate(&relaycommon.RelayInfo{TokenId: 999999}, time.Now())
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeQueryDataError, apiErr.GetErrorCode())
}

func TestDisabledTokenPeriodAttributionIsMemoizedAndRefreshStillReads(t *testing.T) {
	truncate(t)
	const userID, tokenID = 730, 731
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-memo", 0, 100)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"period_type":        "",
		"period_quota_limit": 0,
	}).Error)
	queries := countTokenPeriodStateQueries(t)
	info := &relaycommon.RelayInfo{TokenId: token.Id}

	_, _, apiErr := checkTokenPeriodGate(info, time.Now())
	require.Nil(t, apiErr)
	assert.True(t, info.TokenPeriodAttributionLoaded)
	assert.Zero(t, info.TokenPeriodStartAt)
	assert.Equal(t, int64(1), queries.Load())

	start, err := loadTokenPeriodAttribution(info, false)
	require.NoError(t, err)
	assert.Zero(t, start)
	assert.Equal(t, int64(1), queries.Load(), "a confirmed disabled policy must be memoized")

	_, err = loadTokenPeriodAttribution(info, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), queries.Load(), "WSS refresh must bypass the normal memo")
}

func TestDisabledAttributionPreAndPostConsumeUseOneStateRead(t *testing.T) {
	truncate(t)
	const userID, tokenID = 741, 742
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-direct", 0, 100)
	require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, token.Id).Error)
	queries := countTokenPeriodStateQueries(t)
	info := &relaycommon.RelayInfo{
		TokenId:        token.Id,
		TokenKey:       token.Key,
		TokenUnlimited: true,
		UserId:         userID,
	}

	require.NoError(t, PreConsumeTokenQuota(info, 10))
	assert.Equal(t, int64(1), queries.Load(), "the attribution read also validates the disabled adjustment hint")
	require.NoError(t, PostConsumeQuota(info, 5, 0, false))
	assert.Equal(t, int64(1), queries.Load(), "post-consume reuses the confirmed disabled decision")

	var got model.Token
	require.NoError(t, model.DB.First(&got, token.Id).Error)
	assert.Equal(t, token.RemainQuota-15, got.RemainQuota)
	assert.Equal(t, 15, got.UsedQuota)
	assert.Zero(t, got.PeriodUsedQuota)
}

func TestDisabledBillingSessionReusesOneDecisionAcrossAdjustments(t *testing.T) {
	truncate(t)
	const userID, tokenID, rollbackTokenID = 732, 733, 734
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-session", 0, 100)
	require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, token.Id).Error)
	queries := countTokenPeriodStateQueries(t)
	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000,
		ForcePreConsume: true,
	}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}

	require.Nil(t, session.preConsume(periodBillingContext(), 40))
	require.NoError(t, session.Reserve(60))
	require.NoError(t, session.Settle(50))
	assert.Equal(t, int64(1), queries.Load(), "gate, pre-consume, reserve and settle share the disabled decision")

	var updated model.Token
	require.NoError(t, model.DB.First(&updated, token.Id).Error)
	assert.Equal(t, token.RemainQuota-50, updated.RemainQuota)
	assert.Equal(t, 50, updated.UsedQuota)
	assert.Zero(t, updated.PeriodStartAt)
	assert.Zero(t, updated.PeriodUsedQuota)

	rollbackToken := seedBillingPeriodToken(t, rollbackTokenID, userID, "period-disabled-rollback", 0, 100)
	require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, rollbackToken.Id).Error)
	rollbackInfo := &relaycommon.RelayInfo{
		TokenId:         rollbackToken.Id,
		TokenKey:        rollbackToken.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000,
		ForcePreConsume: true,
	}
	rollback := &BillingSession{relayInfo: rollbackInfo, funding: failingPeriodFunding{}}
	require.NotNil(t, rollback.preConsume(periodBillingContext(), 20))
	assert.Equal(t, int64(2), queries.Load(), "rollback reuses its session's single disabled decision")
	updated = model.Token{}
	require.NoError(t, model.DB.First(&updated, rollbackToken.Id).Error)
	assert.Equal(t, rollbackToken.RemainQuota, updated.RemainQuota)
	assert.Zero(t, updated.PeriodUsedQuota)
}

func TestDisabledBillingSessionReserveCapturesFirstDecision(t *testing.T) {
	truncate(t)
	const userID, tokenID = 743, 744
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-first-reserve", 0, 100)
	require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, token.Id).Error)
	queries := countTokenPeriodStateQueries(t)
	info := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenUnlimited:  true,
		UserId:          userID,
		UserQuota:       10000,
		ForcePreConsume: true,
	}
	session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}

	require.Nil(t, session.preConsume(periodBillingContext(), 0))
	assert.Zero(t, queries.Load())
	require.NoError(t, session.Reserve(20))
	assert.Equal(t, int64(1), queries.Load())
	require.NoError(t, session.Settle(30))
	assert.Equal(t, int64(1), queries.Load(), "settle must reuse the decision first established by Reserve")
	assert.True(t, session.tokenPeriodAdjustmentHintSet)
	assert.True(t, session.tokenPeriodAdjustmentHint.KnownDisabled)
}

type blockingRefundFunding struct {
	entered chan struct{}
	release chan struct{}
}

func (funding *blockingRefundFunding) Source() string       { return BillingSourceWallet }
func (funding *blockingRefundFunding) PreConsume(int) error { return nil }
func (funding *blockingRefundFunding) Settle(int) error     { return nil }
func (funding *blockingRefundFunding) Refund() error {
	close(funding.entered)
	<-funding.release
	return nil
}

func TestBillingSessionAsyncRefundCopiesDisabledDecision(t *testing.T) {
	truncate(t)
	const userID, tokenID = 735, 736
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-refund", 0, 100)
	require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, token.Id).Error)
	queries := countTokenPeriodStateQueries(t)
	funding := &blockingRefundFunding{entered: make(chan struct{}), release: make(chan struct{})}
	info := &relaycommon.RelayInfo{
		TokenId:                      token.Id,
		TokenKey:                     token.Key,
		TokenPeriodAttributionLoaded: true,
		UserId:                       userID,
	}
	session := &BillingSession{
		relayInfo:                    info,
		funding:                      funding,
		preConsumedQuota:             10,
		tokenConsumed:                10,
		tokenPeriodAdjustmentHint:    model.TokenPeriodAdjustmentHint{KnownDisabled: true},
		tokenPeriodAdjustmentHintSet: true,
	}
	updated := make(chan struct{}, 1)
	callbackName := "test:disabled_refund_updated"
	require.NoError(t, model.DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			select {
			case updated <- struct{}{}:
			default:
			}
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	})

	session.Refund(periodBillingContext())
	select {
	case <-funding.entered:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "refund goroutine did not start")
	}
	session.tokenPeriodAdjustmentHint.KnownDisabled = false
	info.TokenPeriodAttributionLoaded = false
	close(funding.release)
	select {
	case <-updated:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "refund token update did not finish")
	}

	assert.Zero(t, queries.Load(), "the goroutine must use its copied KnownDisabled decision")
	var got model.Token
	require.NoError(t, model.DB.First(&got, token.Id).Error)
	assert.Equal(t, token.RemainQuota+10, got.RemainQuota)
}

func TestBillingSessionPolicyChangesRespectAdmissionDecision(t *testing.T) {
	t.Run("active reset is reloaded", func(t *testing.T) {
		truncate(t)
		const userID, tokenID = 737, 738
		seedUser(t, userID, 10000)
		token := seedBillingPeriodToken(t, tokenID, userID, "period-active-reset", 0, 1000)
		info := &relaycommon.RelayInfo{
			TokenId:         token.Id,
			TokenKey:        token.Key,
			TokenUnlimited:  true,
			UserId:          userID,
			UserQuota:       10000,
			ForcePreConsume: true,
		}
		session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}
		require.Nil(t, session.preConsume(periodBillingContext(), 20))

		now := time.Now()
		anchor := common.TokenPeriodAnchorNow(now)
		start, _, ok := common.TokenPeriodBounds(common.TokenPeriodTypeMonth, 0, anchor, now)
		require.True(t, ok)
		require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 3, start, common.TokenPeriodTypeMonth, 0, 1000, anchor, token.Id).Error)
		session.tokenPeriodStartAt = start - 24*60*60
		info.TokenPeriodStartAt = session.tokenPeriodStartAt

		require.NoError(t, session.Settle(30))
		var got model.Token
		require.NoError(t, model.DB.First(&got, token.Id).Error)
		assert.Equal(t, start, got.PeriodStartAt)
		assert.Equal(t, int64(13), got.PeriodUsedQuota)
	})

	t.Run("disabled admission stays legacy after enable", func(t *testing.T) {
		truncate(t)
		const userID, tokenID = 739, 740
		seedUser(t, userID, 10000)
		token := seedBillingPeriodToken(t, tokenID, userID, "period-disabled-enable", 0, 1000)
		require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, 0, "", 0, 0, 0, token.Id).Error)
		info := &relaycommon.RelayInfo{
			TokenId:         token.Id,
			TokenKey:        token.Key,
			TokenUnlimited:  true,
			UserId:          userID,
			UserQuota:       10000,
			ForcePreConsume: true,
		}
		session := &BillingSession{relayInfo: info, funding: &WalletFunding{userId: userID}}
		require.Nil(t, session.preConsume(periodBillingContext(), 20))

		now := time.Now()
		anchor := common.TokenPeriodAnchorNow(now)
		start, _, ok := common.TokenPeriodBounds(common.TokenPeriodTypeMonth, 0, anchor, now)
		require.True(t, ok)
		require.NoError(t, model.DB.Exec(`UPDATE tokens
SET period_used_quota = ?, period_start_at = ?, period_type = ?, period_days = ?, period_quota_limit = ?, period_anchor_at = ?
WHERE id = ?`, 0, start, common.TokenPeriodTypeMonth, 0, 1000, anchor, token.Id).Error)

		require.NoError(t, session.Settle(30))
		var got model.Token
		require.NoError(t, model.DB.First(&got, token.Id).Error)
		assert.Equal(t, start, got.PeriodStartAt)
		assert.Zero(t, got.PeriodUsedQuota)
		assert.Equal(t, token.RemainQuota-30, got.RemainQuota)
	})
}

func TestTokenPeriodAttributionAndGateSkipAndFailureInputs(t *testing.T) {
	for _, info := range []*relaycommon.RelayInfo{
		nil,
		{IsPlayground: true, TokenId: 1},
		{TokenId: 0},
	} {
		start, err := loadTokenPeriodAttribution(info, true)
		assert.Zero(t, start)
		assert.NoError(t, err)
		_, _, apiErr := checkTokenPeriodGate(info, time.Now())
		assert.Nil(t, apiErr)
	}

	oldDB := model.DB
	model.DB = nil
	attributionInfo := &relaycommon.RelayInfo{TokenId: 1, TokenPeriodAttributionLoaded: true}
	_, err := loadTokenPeriodAttribution(attributionInfo, true)
	assert.Error(t, err)
	assert.False(t, attributionInfo.TokenPeriodAttributionLoaded)
	gateInfo := &relaycommon.RelayInfo{TokenId: 1, TokenPeriodAttributionLoaded: true}
	_, _, apiErr := checkTokenPeriodGate(gateInfo, time.Now())
	assert.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeQueryDataError, apiErr.GetErrorCode())
	assert.False(t, gateInfo.TokenPeriodAttributionLoaded)
	model.DB = oldDB

	assert.Nil(t, CheckTokenPeriodGate(&relaycommon.RelayInfo{TokenId: 1}, 0))
}

func TestPublicTokenPeriodGateCoversAdmissionBranches(t *testing.T) {
	truncate(t)
	const userID, tokenID = 722, 723
	seedUser(t, userID, 10000)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-public", 99, 100)
	info := &relaycommon.RelayInfo{TokenId: token.Id}
	assert.Nil(t, CheckTokenPeriodGate(info, 1))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Update("period_used_quota", 100).Error)
	staleCachedDecision := &relaycommon.RelayInfo{
		TokenId:                      token.Id,
		TokenPeriodAttributionLoaded: true,
		TokenPeriodStartAt:           0,
	}
	apiErr := CheckTokenPeriodGate(staleCachedDecision, 1)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeTokenPeriodQuotaExceeded, apiErr.GetErrorCode())
	assert.True(t, staleCachedDecision.TokenPeriodAttributionLoaded)
	assert.Equal(t, token.PeriodStartAt, staleCachedDecision.TokenPeriodStartAt)
}

func TestPeriodRejectionDoesNotFallBackBetweenWalletAndSubscription(t *testing.T) {
	truncate(t)
	const userID, tokenID, subID = 724, 725, 726
	seedUser(t, userID, 0)
	token := seedBillingPeriodToken(t, tokenID, userID, "period-no-fallback", 100, 100)
	seedSubscription(t, subID, userID, 10000, 100)
	info := &relaycommon.RelayInfo{
		TokenId:        token.Id,
		TokenKey:       token.Key,
		TokenUnlimited: true,
		UserId:         userID,
		RequestId:      "period-no-fallback-request",
		UserSetting:    dto.UserSetting{BillingPreference: "wallet_first"},
	}

	_, apiErr := NewBillingSession(periodBillingContext(), info, 50, 50)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeTokenPeriodQuotaExceeded, apiErr.GetErrorCode())
	assert.Equal(t, int64(100), getSubscriptionUsed(t, subID), "period rejection must not consume the fallback subscription")
}

func TestBillingSessionPeriodGateHelperNoopBranches(t *testing.T) {
	session := &BillingSession{}
	assert.Nil(t, session.checkPeriodGateAfterTrust(10))
	session = &BillingSession{relayInfo: &relaycommon.RelayInfo{IsPlayground: true}}
	assert.Nil(t, session.checkPeriodGateAfterTrust(10))
	session = &BillingSession{relayInfo: &relaycommon.RelayInfo{TokenId: 1}, periodGateChecked: true}
	assert.Nil(t, session.checkPeriodGateAfterTrust(10))
}

func TestMidjourneyRefundHelperNoopAndMissingToken(t *testing.T) {
	assert.NoError(t, RefundMidjourneyTokenQuota(context.Background(), nil))
	assert.NoError(t, RefundMidjourneyTokenQuota(context.Background(), &model.Midjourney{Quota: 0}))
	err := RefundMidjourneyTokenQuota(context.Background(), &model.Midjourney{TokenId: 999999, Quota: 1})
	assert.Error(t, err)
}

func operationSettingUSDExchangeRate() float64         { return operation_setting.USDExchangeRate }
func setOperationSettingUSDExchangeRate(value float64) { operation_setting.USDExchangeRate = value }

// alpha_search (controller/relay_alpha_search.go) 与现代 task 一样走 ForcePreConsume，
// 提交即占用，必须同样受周期限额门控。
func TestForcePreConsumeBillingGatesTokenPeriod(t *testing.T) {
	truncate(t)
	const userID, allowedID, blockedID = 727, 728, 729
	seedUser(t, userID, common.GetTrustQuota()+100000)
	allowed := seedBillingPeriodToken(t, allowedID, userID, "period-force-ok", 99, 100)
	blocked := seedBillingPeriodToken(t, blockedID, userID, "period-force-blocked", 100, 100)

	forceInfo := func(token *model.Token) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			TokenId:         token.Id,
			TokenKey:        token.Key,
			TokenUnlimited:  true,
			UserId:          userID,
			UserQuota:       common.GetTrustQuota() + 100000,
			ForcePreConsume: true,
		}
	}

	admitted := forceInfo(allowed)
	require.Nil(t, PreConsumeBilling(periodBillingContext(), 10, 10, admitted))
	assert.Equal(t, allowed.PeriodStartAt, admitted.TokenPeriodStartAt, "attribution is captured for later settle/refund")

	rejected := forceInfo(blocked)
	apiErr := PreConsumeBilling(periodBillingContext(), 10, 10, rejected)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeTokenPeriodQuotaExceeded, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.True(t, apiErr.IsSkipRetry())
	assert.Nil(t, rejected.Billing, "a rejected request must not carry a billing session")
}

func TestTaskErrorFromAPIErrorMarksOnlyPeriodRejectionAsLocal(t *testing.T) {
	assert.Nil(t, TaskErrorFromAPIError(nil))

	periodErr := TaskErrorFromAPIError(types.NewErrorWithStatusCode(
		fmt.Errorf("period exhausted"),
		types.ErrorCodeTokenPeriodQuotaExceeded,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	))
	require.NotNil(t, periodErr)
	assert.True(t, periodErr.LocalError, "period rejection must not retry across channels nor mark the channel unhealthy")

	upstreamErr := TaskErrorFromAPIError(types.NewErrorWithStatusCode(
		fmt.Errorf("bad upstream"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
		types.ErrOptionWithSkipRetry(),
	))
	require.NotNil(t, upstreamErr)
	assert.False(t, upstreamErr.LocalError, "other skip-retry errors keep their existing channel-error semantics")
}
