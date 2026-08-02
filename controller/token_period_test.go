package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTokenPeriodRequestNormalizesCNYAndIgnoresStateFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	oldRate := common.QuotaPerUnit
	oldUSD := operation_setting.USDExchangeRate
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 5
	t.Cleanup(func() {
		common.QuotaPerUnit = oldRate
		operation_setting.USDExchangeRate = oldUSD
	})

	body := map[string]any{
		"name":               "period-token",
		"expired_time":       -1,
		"remain_quota":       100,
		"unlimited_quota":    true,
		"period_type":        common.TokenPeriodTypeDays,
		"period_days":        3,
		"period_limit_unit":  "cny",
		"period_limit_value": "10.00",
		"period_used_quota":  999,
		"period_start_at":    123,
		"period_anchor_at":   456,
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token", body, 1)
	AddToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var token model.Token
	require.NoError(t, db.Where("name = ?", "period-token").First(&token).Error)
	assert.Equal(t, int64(1000000), token.PeriodQuotaLimit)
	assert.Equal(t, int64(0), token.PeriodUsedQuota)
	assert.NotZero(t, token.PeriodAnchorAt)
	assert.NotZero(t, token.PeriodStartAt)
}

func TestUpdateTokenStatusOnlyDoesNotChangePeriodState(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	start := common.TokenPeriodAnchorNow(time.Now())
	token := seedToken(t, db, 1, "status-period", "status-period-key")
	token.PeriodType = common.TokenPeriodTypeDays
	token.PeriodDays = 2
	token.PeriodQuotaLimit = 100
	token.PeriodLimitUnit = "quota"
	token.PeriodAnchorAt = start
	token.PeriodStartAt = start
	token.PeriodUsedQuota = 37
	require.NoError(t, db.Save(token).Error)

	body := map[string]any{
		"id":                 token.Id,
		"status":             common.TokenStatusDisabled,
		"period_type":        common.TokenPeriodTypeMonth,
		"period_days":        3650,
		"period_limit_unit":  "cny",
		"period_limit_value": "99",
		"period_used_quota":  9999,
		"period_start_at":    9999,
		"period_anchor_at":   9999,
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token?status_only=1", body, 1)
	UpdateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, updated.Status)
	assert.Equal(t, token.PeriodType, updated.PeriodType)
	assert.Equal(t, token.PeriodDays, updated.PeriodDays)
	assert.Equal(t, token.PeriodQuotaLimit, updated.PeriodQuotaLimit)
	assert.Equal(t, token.PeriodAnchorAt, updated.PeriodAnchorAt)
	assert.Equal(t, token.PeriodStartAt, updated.PeriodStartAt)
	assert.Equal(t, token.PeriodUsedQuota, updated.PeriodUsedQuota)
}

func TestTokenPeriodConfigurationChangeResetsOnlyShapeChanges(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "change-period", "change-period-key")
	anchor := common.TokenPeriodAnchorNow(time.Now())
	start, _, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 2, anchor, time.Now())
	require.True(t, ok)
	token.PeriodType = common.TokenPeriodTypeDays
	token.PeriodDays = 2
	token.PeriodQuotaLimit = 100
	token.PeriodLimitUnit = "quota"
	token.PeriodAnchorAt = anchor
	token.PeriodStartAt = start
	token.PeriodUsedQuota = 40
	require.NoError(t, db.Save(token).Error)

	limitBody := map[string]any{
		"id":                 token.Id,
		"name":               token.Name,
		"expired_time":       -1,
		"remain_quota":       100,
		"unlimited_quota":    true,
		"period_type":        common.TokenPeriodTypeDays,
		"period_days":        2,
		"period_limit_unit":  "quota",
		"period_limit_value": "150",
		"period_used_quota":  9999,
		"period_start_at":    9999,
		"period_anchor_at":   9999,
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token", limitBody, 1)
	UpdateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, decodeAPIResponse(t, recorder).Success)

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, int64(150), updated.PeriodQuotaLimit)
	assert.Equal(t, int64(40), updated.PeriodUsedQuota)
	assert.Equal(t, start, updated.PeriodStartAt)

	shapeBody := limitBody
	shapeBody["period_days"] = 3
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token", shapeBody, 1)
	UpdateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, decodeAPIResponse(t, recorder).Success)

	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, 3, updated.PeriodDays)
	assert.Zero(t, updated.PeriodUsedQuota)
	assert.Equal(t, start, updated.PeriodStartAt)
}

func TestNormalizeTokenPeriodValidatesBoundariesAndConversion(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	oldUSD := operation_setting.USDExchangeRate
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 5
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldUSD
	})

	tests := []struct {
		name      string
		request   dto.TokenRequest
		wantLimit int64
		wantDays  int
		wantErr   bool
	}{
		{
			name:      "raw quota minimum",
			request:   dto.TokenRequest{PeriodType: common.TokenPeriodTypeDays, PeriodDays: 1, PeriodLimitUnit: "quota", PeriodLimitValue: "1"},
			wantLimit: 1,
			wantDays:  1,
		},
		{
			name:      "raw quota maximum",
			request:   dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "quota", PeriodLimitValue: "2147483647"},
			wantLimit: int64(common.MaxQuota),
		},
		{
			name:      "cny rounds half away from zero",
			request:   dto.TokenRequest{PeriodType: common.TokenPeriodTypeWeek, PeriodLimitUnit: "cny", PeriodLimitValue: "0.000015"},
			wantLimit: 2,
		},
		{
			name:      "week ignores days",
			request:   dto.TokenRequest{PeriodType: common.TokenPeriodTypeWeek, PeriodDays: 3651, PeriodLimitUnit: "quota", PeriodLimitValue: "2"},
			wantLimit: 2,
		},
		{name: "invalid type", request: dto.TokenRequest{PeriodType: "year", PeriodLimitUnit: "quota", PeriodLimitValue: "1"}, wantErr: true},
		{name: "invalid unit", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "usd", PeriodLimitValue: "1"}, wantErr: true},
		{name: "days zero", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeDays, PeriodDays: 0, PeriodLimitUnit: "quota", PeriodLimitValue: "1"}, wantErr: true},
		{name: "days above maximum", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeDays, PeriodDays: 3651, PeriodLimitUnit: "quota", PeriodLimitValue: "1"}, wantErr: true},
		{name: "negative value", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "quota", PeriodLimitValue: "-1"}, wantErr: true},
		{name: "fractional raw quota", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "quota", PeriodLimitValue: "1.5"}, wantErr: true},
		{name: "raw quota above maximum", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "quota", PeriodLimitValue: "2147483648"}, wantErr: true},
		{name: "cny conversion clamps", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "cny", PeriodLimitValue: "1000000000000000000"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := normalizeTokenPeriod(tt.request)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantLimit, config.limit)
			assert.Equal(t, tt.wantDays, config.periodDays)
		})
	}
}

func TestNormalizeTokenPeriodDisabledAndMissingValueCases(t *testing.T) {
	tests := []struct {
		name    string
		request dto.TokenRequest
		wantErr bool
	}{
		{name: "disabled omitted fields", request: dto.TokenRequest{}},
		{name: "disabled explicit zero", request: dto.TokenRequest{PeriodLimitUnit: "cny", PeriodLimitValue: "0"}},
		{name: "disabled invalid unit", request: dto.TokenRequest{PeriodLimitUnit: "usd"}, wantErr: true},
		{name: "disabled nonzero value", request: dto.TokenRequest{PeriodLimitUnit: "quota", PeriodLimitValue: "1"}, wantErr: true},
		{name: "disabled malformed value", request: dto.TokenRequest{PeriodLimitUnit: "quota", PeriodLimitValue: "invalid"}, wantErr: true},
		{name: "enabled missing value", request: dto.TokenRequest{PeriodType: common.TokenPeriodTypeMonth, PeriodLimitUnit: "quota"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeTokenPeriod(tt.request)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	oldUSD := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 0
	t.Cleanup(func() { operation_setting.USDExchangeRate = oldUSD })
	_, err := normalizeTokenPeriod(dto.TokenRequest{
		PeriodType:       common.TokenPeriodTypeMonth,
		PeriodLimitUnit:  "cny",
		PeriodLimitValue: "1",
	})
	require.Error(t, err)
}

func TestTokenPeriodConfigNeedsResetRules(t *testing.T) {
	active := &model.Token{
		PeriodType:       common.TokenPeriodTypeDays,
		PeriodDays:       3,
		PeriodQuotaLimit: 100,
	}
	same := normalizedTokenPeriod{periodType: common.TokenPeriodTypeDays, periodDays: 3, limit: 200, unit: "cny"}
	assert.False(t, tokenPeriodConfigNeedsReset(active, same))
	assert.True(t, tokenPeriodConfigNeedsReset(active, normalizedTokenPeriod{}))
	assert.True(t, tokenPeriodConfigNeedsReset(nil, same))
	assert.True(t, tokenPeriodConfigNeedsReset(&model.Token{}, same))
	assert.True(t, tokenPeriodConfigNeedsReset(active, normalizedTokenPeriod{
		periodType: common.TokenPeriodTypeMonth,
		limit:      200,
		unit:       "quota",
	}))
}

func TestApplyAndEnrichTokenPeriodConfigDefensiveBranches(t *testing.T) {
	now := time.Now()
	require.Error(t, applyTokenPeriodConfig(nil, normalizedTokenPeriod{}, now))
	enrichTokenPeriodResponse(nil)

	disabled := &model.Token{PeriodStartAt: 10, PeriodUsedQuota: 20}
	enrichTokenPeriodResponse(disabled)
	assert.Zero(t, disabled.PeriodUsedQuota)
	assert.Zero(t, disabled.PeriodResetAt)
	assert.Zero(t, disabled.PeriodRemainingQuota)

	anchor := common.TokenPeriodAnchorNow(now)
	start, resetAt, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 1, anchor, now)
	require.True(t, ok)
	active := &model.Token{
		PeriodType:       common.TokenPeriodTypeDays,
		PeriodDays:       1,
		PeriodQuotaLimit: 10,
		PeriodLimitUnit:  "quota",
		PeriodAnchorAt:   anchor,
		PeriodStartAt:    start,
		PeriodUsedQuota:  15,
	}
	enrichTokenPeriodResponse(active)
	assert.Equal(t, int64(15), active.PeriodUsedQuota)
	assert.Equal(t, resetAt, active.PeriodResetAt)
	assert.Zero(t, active.PeriodRemainingQuota)
}

func TestAddTokenPeriodValidationReturnsBadRequest(t *testing.T) {
	setupTokenControllerTestDB(t)
	body := map[string]any{
		"name":               "invalid-period",
		"unlimited_quota":    true,
		"period_type":        common.TokenPeriodTypeDays,
		"period_days":        0,
		"period_limit_unit":  "quota",
		"period_limit_value": "10",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token", body, 1)
	AddToken(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.False(t, decodeAPIResponse(t, recorder).Success)
}

func TestGetAllTokensReturnsEffectivePeriodStateWithoutWriting(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "stale-period", "stale-period-key")
	anchor := common.TokenPeriodAnchorNow(time.Now())
	currentStart, resetAt, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 1, anchor, time.Now())
	require.True(t, ok)
	token.PeriodType = common.TokenPeriodTypeDays
	token.PeriodDays = 1
	token.PeriodQuotaLimit = 100
	token.PeriodLimitUnit = "quota"
	token.PeriodAnchorAt = anchor
	token.PeriodStartAt = currentStart - 24*60*60
	token.PeriodUsedQuota = 80
	require.NoError(t, db.Save(token).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 1)
	GetAllTokens(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var page struct {
		Items []model.Token `json:"items"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &page))
	require.Len(t, page.Items, 1)
	assert.Zero(t, page.Items[0].PeriodUsedQuota)
	assert.Equal(t, int64(100), page.Items[0].PeriodRemainingQuota)
	assert.Equal(t, resetAt, page.Items[0].PeriodResetAt)

	var persisted model.Token
	require.NoError(t, db.First(&persisted, token.Id).Error)
	assert.Equal(t, currentStart-24*60*60, persisted.PeriodStartAt)
	assert.Equal(t, int64(80), persisted.PeriodUsedQuota)
}
