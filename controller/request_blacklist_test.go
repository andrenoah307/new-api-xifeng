package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayRequestBlacklistResponseContainsOnlyPublicPolicyInformation(t *testing.T) {
	previousConfig := setting.RequestBlacklist2JSONString()
	previousCountToken := constant.CountToken
	previousSensitiveEnabled := setting.CheckSensitiveEnabled
	previousPromptSensitiveEnabled := setting.CheckSensitiveOnPromptEnabled
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateRequestBlacklistByJSONString(previousConfig))
		constant.CountToken = previousCountToken
		setting.CheckSensitiveEnabled = previousSensitiveEnabled
		setting.CheckSensitiveOnPromptEnabled = previousPromptSensitiveEnabled
	})

	config := setting.RequestBlacklistConfig{
		Mode:            "enforce",
		EnabledGroups:   []string{"private-group"},
		ContentWords:    []string{"classified-target"},
		BlockMessage:    "Request rejected by gateway policy",
		BlockStatusCode: http.StatusBadRequest,
	}
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateRequestBlacklistByJSONString(string(configJSON)))
	constant.CountToken = false
	setting.CheckSensitiveEnabled = false
	setting.CheckSensitiveOnPromptEnabled = false

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"safe-model","messages":[{"role":"user","content":"inspect classified-target now"}]}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	c.Set(common.RequestIdKey, "request-blacklist-123")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "safe-model")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "private-group")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "private-group")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "private-group")
	common.SetContextKey(c, constant.ContextKeyUserId, 41)
	common.SetContextKey(c, constant.ContextKeyTokenId, 73)
	common.SetContextKey(c, constant.ContextKeyChannelId, 991337)
	common.SetContextKey(c, constant.ContextKeyChannelName, "internal-channel-name")
	common.SetContextKey(c, constant.ContextKeyChannelType, 1)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "upstream-secret")
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://upstream.invalid")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	Relay(c, types.RelayFormatOpenAI)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "Request rejected by gateway policy")
	assert.Contains(t, body, "request-blacklist-123")
	assert.Contains(t, body, string(types.ErrorCodeRequestContentBlocked))
	assert.NotContains(t, body, "private-group")
	assert.NotContains(t, body, "internal-channel-name")
	assert.NotContains(t, body, "991337")
	assert.NotContains(t, body, "classified-target")
	assert.NotContains(t, body, "upstream-secret")
	assert.NotContains(t, body, "price")
}

func TestUpdateOptionRejectsInvalidRequestBlacklistBeforePersistence(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	payload, err := common.Marshal(OptionUpdateRequest{
		Key:   "RequestBlacklist",
		Value: `{"mode":"enforce","block_status_code":500}`,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)

	UpdateOption(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "RequestBlacklist").Count(&count).Error)
	assert.Zero(t, count)
}
