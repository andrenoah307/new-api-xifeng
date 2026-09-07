package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRedemptionName(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	confirmPaymentComplianceForTest(t)

	tests := []struct {
		name       string
		redemption string
		wantValid  bool
		wantError  string
	}{
		{name: "Chinese", redemption: "兑换码", wantValid: true},
		{name: "English", redemption: "promo", wantValid: true},
		{name: "spaces", redemption: "promo code", wantValid: true},
		{name: "emoji", redemption: "促销🎁", wantValid: true},
		{name: "tab", redemption: "promo\tcode", wantError: i18n.MsgRedemptionNameInvalidChars},
		{name: "line feed", redemption: "promo\ncode", wantError: i18n.MsgRedemptionNameInvalidChars},
		{name: "carriage return", redemption: "promo\rcode", wantError: i18n.MsgRedemptionNameInvalidChars},
		{name: "null", redemption: "promo\x00code", wantError: i18n.MsgRedemptionNameInvalidChars},
		{name: "empty", redemption: "", wantError: i18n.MsgRedemptionNameLength},
		{name: "too long", redemption: "123456789012345678901", wantError: i18n.MsgRedemptionNameLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := int64(0)
			require.NoError(t, db.Model(&model.Redemption{}).Count(&before).Error)

			response := callUserRedemptionController(t, AddRedemption, http.MethodPost, "/api/redemption", map[string]any{
				"name":  tt.redemption,
				"count": 1,
				"quota": 100,
			}, 1)

			if tt.wantValid {
				assert.True(t, response.Success, response.Message)
				assert.Empty(t, response.Message)
			} else {
				assert.False(t, response.Success)
				assert.Equal(t, i18n.Translate(i18n.LangEn, tt.wantError), response.Message)
			}

			var after int64
			require.NoError(t, db.Model(&model.Redemption{}).Count(&after).Error)
			if tt.wantValid {
				assert.Equal(t, before+1, after)
			} else {
				assert.Equal(t, before, after)
			}
		})
	}
}

func TestUpdateRedemptionName(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)

	tests := []struct {
		name           string
		key            string
		updatedName    string
		statusOnly     string
		status         int
		wantSuccess    bool
		wantMessage    string
		wantStoredName string
	}{
		{
			name:           "valid name updates",
			key:            "30000000000000000000000000000001",
			updatedName:    "updated-name",
			wantSuccess:    true,
			wantStoredName: "updated-name",
		},
		{
			name:           "tab is rejected",
			key:            "30000000000000000000000000000002",
			updatedName:    "invalid\tname",
			wantMessage:    i18n.MsgRedemptionNameInvalidChars,
			wantStoredName: "original-name",
		},
		{
			name:           "line feed is rejected",
			key:            "30000000000000000000000000000003",
			updatedName:    "invalid\nname",
			wantMessage:    i18n.MsgRedemptionNameInvalidChars,
			wantStoredName: "original-name",
		},
		{
			name:           "null is rejected",
			key:            "30000000000000000000000000000004",
			updatedName:    "invalid\x00name",
			wantMessage:    i18n.MsgRedemptionNameInvalidChars,
			wantStoredName: "original-name",
		},
		{
			name:           "status only ignores invalid name",
			key:            "30000000000000000000000000000005",
			updatedName:    "invalid\tname",
			statusOnly:     "1",
			status:         common.RedemptionCodeStatusDisabled,
			wantSuccess:    true,
			wantStoredName: "original-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redemption := &model.Redemption{
				Name:   "original-name",
				Key:    tt.key,
				Status: common.RedemptionCodeStatusEnabled,
				Quota:  100,
			}
			require.NoError(t, db.Create(redemption).Error)

			response := callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/?status_only="+tt.statusOnly, map[string]any{
				"id":           redemption.Id,
				"name":         tt.updatedName,
				"quota":        100,
				"expired_time": int64(0),
				"status":       tt.status,
			}, 1)

			if tt.wantSuccess {
				assert.True(t, response.Success, response.Message)
				assert.Empty(t, response.Message)
			} else {
				assert.False(t, response.Success)
				assert.Equal(t, i18n.Translate(i18n.LangEn, tt.wantMessage), response.Message)
			}

			var got model.Redemption
			require.NoError(t, db.First(&got, redemption.Id).Error)
			assert.Equal(t, tt.wantStoredName, got.Name)
			if tt.statusOnly != "" {
				assert.Equal(t, tt.status, got.Status)
			}
		})
	}
}
