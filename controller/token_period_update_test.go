package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUpdateRequestPeriodFieldPresence(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantPresent bool
		wantType    string
		wantDays    int
		wantUnit    string
		wantValue   string
	}{
		{
			name: "omitted",
			body: `{"id":7}`,
		},
		{
			name: "explicit null",
			body: `{"id":7,"period_type":null,"period_days":null,"period_limit_unit":null,"period_limit_value":null}`,
		},
		{
			name:        "explicit values including empty type",
			body:        `{"id":7,"period_type":"","period_days":0,"period_limit_unit":"cny","period_limit_value":"0"}`,
			wantPresent: true,
			wantType:    "",
			wantDays:    0,
			wantUnit:    "cny",
			wantValue:   "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request dto.TokenUpdateRequest
			require.NoError(t, common.Unmarshal([]byte(tt.body), &request))
			assert.Equal(t, 7, request.Id)
			if !tt.wantPresent {
				assert.Nil(t, request.PeriodType)
				assert.Nil(t, request.PeriodDays)
				assert.Nil(t, request.PeriodLimitUnit)
				assert.Nil(t, request.PeriodLimitValue)
				return
			}
			require.NotNil(t, request.PeriodType)
			require.NotNil(t, request.PeriodDays)
			require.NotNil(t, request.PeriodLimitUnit)
			require.NotNil(t, request.PeriodLimitValue)
			assert.Equal(t, tt.wantType, *request.PeriodType)
			assert.Equal(t, tt.wantDays, *request.PeriodDays)
			assert.Equal(t, tt.wantUnit, *request.PeriodLimitUnit)
			assert.Equal(t, tt.wantValue, *request.PeriodLimitValue)
		})
	}
}

func TestUpdateTokenPeriodPresenceSemantics(t *testing.T) {
	tests := []struct {
		name       string
		usedQuota  int64
		periodBody map[string]any
	}{
		{
			name: "omitted fields preserve unused period",
		},
		{
			name:      "omitted fields preserve consumed quota",
			usedQuota: 37,
		},
		{
			name:      "explicit null fields are omitted",
			usedQuota: 23,
			periodBody: map[string]any{
				"period_type":        nil,
				"period_days":        nil,
				"period_limit_unit":  nil,
				"period_limit_value": nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTokenControllerTestDB(t)
			token := seedToken(t, db, 1, "before", "presence-"+tt.name)
			anchor := common.TokenPeriodAnchorNow(time.Now())
			start, _, ok := common.TokenPeriodBounds(common.TokenPeriodTypeDays, 2, anchor, time.Now())
			require.True(t, ok)
			token.PeriodType = common.TokenPeriodTypeDays
			token.PeriodDays = 2
			token.PeriodQuotaLimit = 400
			token.PeriodLimitUnit = "quota"
			token.PeriodAnchorAt = anchor
			token.PeriodStartAt = start
			token.PeriodUsedQuota = tt.usedQuota
			require.NoError(t, db.Save(token).Error)

			body := map[string]any{
				"id":              token.Id,
				"name":            "after",
				"expired_time":    -1,
				"remain_quota":    275,
				"unlimited_quota": true,
			}
			for key, value := range tt.periodBody {
				body[key] = value
			}

			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token", body, 1)
			UpdateToken(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			response := decodeAPIResponse(t, recorder)
			require.True(t, response.Success, response.Message)

			var updated model.Token
			require.NoError(t, db.First(&updated, token.Id).Error)
			assert.Equal(t, "after", updated.Name)
			assert.Equal(t, 275, updated.RemainQuota)
			assert.Equal(t, token.PeriodType, updated.PeriodType)
			assert.Equal(t, token.PeriodDays, updated.PeriodDays)
			assert.Equal(t, token.PeriodQuotaLimit, updated.PeriodQuotaLimit)
			assert.Equal(t, token.PeriodLimitUnit, updated.PeriodLimitUnit)
			assert.Equal(t, token.PeriodAnchorAt, updated.PeriodAnchorAt)
			assert.Equal(t, token.PeriodStartAt, updated.PeriodStartAt)
			assert.Equal(t, token.PeriodUsedQuota, updated.PeriodUsedQuota)

			var responseToken model.Token
			require.NoError(t, common.Unmarshal(response.Data, &responseToken))
			assert.Equal(t, token.PeriodType, responseToken.PeriodType)
			assert.Equal(t, token.PeriodDays, responseToken.PeriodDays)
			assert.Equal(t, token.PeriodQuotaLimit, responseToken.PeriodQuotaLimit)
			assert.Equal(t, token.PeriodLimitUnit, responseToken.PeriodLimitUnit)
			assert.Equal(t, token.PeriodAnchorAt, responseToken.PeriodAnchorAt)
			assert.Equal(t, token.PeriodStartAt, responseToken.PeriodStartAt)
			assert.Equal(t, token.PeriodUsedQuota, responseToken.PeriodUsedQuota)
		})
	}
}

func TestUpdateTokenExplicitPeriodDisableClearsConfigurationAndState(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "disable-period", "disable-period-key")
	token.PeriodType = common.TokenPeriodTypeMonth
	token.PeriodDays = 0
	token.PeriodQuotaLimit = 500
	token.PeriodLimitUnit = "cny"
	token.PeriodAnchorAt = 100
	token.PeriodStartAt = 200
	token.PeriodUsedQuota = 300
	require.NoError(t, db.Save(token).Error)

	body := map[string]any{
		"id":                 token.Id,
		"name":               token.Name,
		"expired_time":       -1,
		"remain_quota":       100,
		"unlimited_quota":    true,
		"period_type":        "",
		"period_days":        0,
		"period_limit_unit":  "cny",
		"period_limit_value": "0",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token", body, 1)
	UpdateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, decodeAPIResponse(t, recorder).Success)

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Empty(t, updated.PeriodType)
	assert.Zero(t, updated.PeriodDays)
	assert.Zero(t, updated.PeriodQuotaLimit)
	assert.Empty(t, updated.PeriodLimitUnit)
	assert.Zero(t, updated.PeriodAnchorAt)
	assert.Zero(t, updated.PeriodStartAt)
	assert.Zero(t, updated.PeriodUsedQuota)
}

func TestUpdateTokenRejectsIncompletePeriodFields(t *testing.T) {
	tests := []struct {
		name        string
		periodBody  map[string]any
		wantMessage string
	}{
		{
			name: "enabled month missing value",
			periodBody: map[string]any{
				"period_type":       common.TokenPeriodTypeMonth,
				"period_limit_unit": "quota",
			},
			wantMessage: "周期限额字段必须完整提供",
		},
		{
			name: "enabled month missing unit",
			periodBody: map[string]any{
				"period_type":        common.TokenPeriodTypeMonth,
				"period_limit_value": "100",
			},
			wantMessage: "周期限额字段必须完整提供",
		},
		{
			name: "enabled days missing days",
			periodBody: map[string]any{
				"period_type":        common.TokenPeriodTypeDays,
				"period_limit_unit":  "quota",
				"period_limit_value": "100",
			},
			wantMessage: "周期限额字段必须完整提供",
		},
		{
			name: "value without type",
			periodBody: map[string]any{
				"period_limit_value": "100",
			},
			wantMessage: "period_type 必须与周期限额字段一起提供",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTokenControllerTestDB(t)
			token := seedToken(t, db, 1, tt.name, "invalid-"+tt.name)
			body := map[string]any{
				"id":              token.Id,
				"name":            token.Name,
				"expired_time":    -1,
				"remain_quota":    100,
				"unlimited_quota": true,
			}
			for key, value := range tt.periodBody {
				body[key] = value
			}

			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token", body, 1)
			UpdateToken(ctx)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			response := decodeAPIResponse(t, recorder)
			assert.False(t, response.Success)
			assert.Equal(t, tt.wantMessage, response.Message)
		})
	}
}

func TestUpdateTokenStatusOnlyIgnoresIncompletePeriodFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "status-incomplete", "status-incomplete-key")
	token.PeriodType = common.TokenPeriodTypeMonth
	token.PeriodQuotaLimit = 100
	token.PeriodLimitUnit = "quota"
	token.PeriodAnchorAt = 10
	token.PeriodStartAt = 20
	token.PeriodUsedQuota = 30
	require.NoError(t, db.Save(token).Error)

	body := map[string]any{
		"id":                 token.Id,
		"status":             common.TokenStatusDisabled,
		"period_limit_value": "invalid without period_type",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token?status_only=true", body, 1)
	UpdateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, decodeAPIResponse(t, recorder).Success)

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, updated.Status)
	assert.Equal(t, token.PeriodType, updated.PeriodType)
	assert.Equal(t, token.PeriodDays, updated.PeriodDays)
	assert.Equal(t, token.PeriodQuotaLimit, updated.PeriodQuotaLimit)
	assert.Equal(t, token.PeriodLimitUnit, updated.PeriodLimitUnit)
	assert.Equal(t, token.PeriodAnchorAt, updated.PeriodAnchorAt)
	assert.Equal(t, token.PeriodStartAt, updated.PeriodStartAt)
	assert.Equal(t, token.PeriodUsedQuota, updated.PeriodUsedQuota)
}
