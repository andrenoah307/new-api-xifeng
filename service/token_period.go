package service

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

var tokenPeriodUTC8 = time.FixedZone("UTC+8", 8*60*60)

// tokenPeriodCNY formats quota in the fixed RMB display unit required by the
// period-limit error contract. It intentionally ignores the site's display
// preference (tokens/USD/custom).
func tokenPeriodCNY(quota int64) string {
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) ||
		operation_setting.USDExchangeRate <= 0 || math.IsNaN(operation_setting.USDExchangeRate) || math.IsInf(operation_setting.USDExchangeRate, 0) {
		return "¥0.00"
	}
	value := decimal.NewFromInt(quota).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(operation_setting.USDExchangeRate)).
		Round(2)
	return "¥" + value.StringFixed(2)
}

func tokenPeriodResetText(resetAt int64) string {
	if resetAt <= 0 {
		return "未知"
	}
	return time.Unix(resetAt, 0).In(tokenPeriodUTC8).Format("2006-01-02 15:04:05 MST")
}

// buildTokenPeriodQuotaExceededMessage is kept free of request/channel
// context so the client-facing string cannot accidentally disclose group or
// upstream details.
func buildTokenPeriodQuotaExceededMessage(state *model.TokenPeriodState, used int64, now time.Time) string {
	if state == nil {
		return "令牌周期限额已用尽"
	}
	return fmt.Sprintf("令牌周期限额已用尽：本周期已用 %s（%d quota），上限 %s（%d quota）；下次重置时间 %s。",
		tokenPeriodCNY(used), used,
		tokenPeriodCNY(state.Limit), state.Limit,
		tokenPeriodResetText(state.ResetAt(now)))
}

func tokenPeriodQuotaExceededError(state *model.TokenPeriodState, used int64, now time.Time) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(buildTokenPeriodQuotaExceededMessage(state, used, now)),
		types.ErrorCodeTokenPeriodQuotaExceeded,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

// loadTokenPeriodAttribution reads the authoritative period state and stores
// the current bucket on RelayInfo. refresh=true is used for each WSS segment;
// normal request lifecycles reuse the admission bucket once captured.
func loadTokenPeriodAttribution(info *relaycommon.RelayInfo, refresh bool) (int64, error) {
	if info == nil || info.IsPlayground || info.TokenId <= 0 {
		return 0, nil
	}
	if !refresh && info.TokenPeriodStartAt > 0 {
		return info.TokenPeriodStartAt, nil
	}
	state, err := model.LoadTokenPeriodState(info.TokenId)
	if err != nil {
		return 0, err
	}
	if !state.PeriodLimitEnabled() {
		info.TokenPeriodStartAt = 0
		return 0, nil
	}
	start := state.CurrentStart(time.Now())
	if start <= 0 {
		return 0, errors.New("令牌周期配置无效")
	}
	info.TokenPeriodStartAt = start
	return start, nil
}

// checkTokenPeriodGate implements E3 soft gating: only the already-used
// amount is compared with the limit. The current request estimate is never
// added to the comparison, so accepted requests may temporarily overshoot.
func checkTokenPeriodGate(info *relaycommon.RelayInfo, now time.Time) (*model.TokenPeriodState, int64, *types.NewAPIError) {
	if info == nil || info.IsPlayground || info.TokenId <= 0 {
		return nil, 0, nil
	}
	state, err := model.LoadTokenPeriodState(info.TokenId)
	if err != nil {
		return nil, 0, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	if !state.PeriodLimitEnabled() {
		info.TokenPeriodStartAt = 0
		return state, 0, nil
	}
	currentStart := state.CurrentStart(now)
	if currentStart <= 0 {
		return nil, 0, types.NewError(errors.New("令牌周期配置无效"), types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	used := state.EffectiveUsed(now)
	info.TokenPeriodStartAt = currentStart
	if used >= state.Limit {
		return state, used, tokenPeriodQuotaExceededError(state, used, now)
	}
	return state, used, nil
}

// CheckTokenPeriodGate is the public admission hook for legacy paths that do
// not create a BillingSession (for example Midjourney and WSS segments).
// Callers pass the charge estimate only to decide whether a zero-cost request
// should skip the lookup; it is never added to the used-vs-limit comparison.
func CheckTokenPeriodGate(info *relaycommon.RelayInfo, quota int) *types.NewAPIError {
	if quota <= 0 || info == nil || info.IsPlayground {
		return nil
	}
	_, _, apiErr := checkTokenPeriodGate(info, time.Now())
	return apiErr
}
