package controller

import (
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserRedemptionControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDB := common.MainDatabaseType()
	originalLogDatabase := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	require.NoError(t, i18n.Init())

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Redemption{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDB, originalLogDatabase)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

type controllerUpdateResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func callUserRedemptionController(t *testing.T, handler gin.HandlerFunc, method string, path string, body any, userID int) controllerUpdateResponse {
	t.Helper()
	var payload bytes.Reader
	if body != nil {
		encoded, err := common.Marshal(body)
		require.NoError(t, err)
		payload = *bytes.NewReader(encoded)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == nil {
		ctx.Request = httptest.NewRequest(method, path, nil)
	} else {
		ctx.Request = httptest.NewRequest(method, path, &payload)
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	handler(ctx)
	var response controllerUpdateResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func callRawUserRedemptionController(t *testing.T, handler gin.HandlerFunc, method string, path string, body string, userID int) controllerUpdateResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	handler(ctx)
	var response controllerUpdateResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGenerateAccessTokenDoesNotWriteDisabledUser(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	oldToken := "disabled-token"
	user := &model.User{
		Username:    "controller-disabled-token",
		Password:    "hashed",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusDisabled,
		Group:       "default",
		AccessToken: &oldToken,
	}
	require.NoError(t, model.DB.Create(user).Error)

	response := callUserRedemptionController(t, GenerateAccessToken, http.MethodGet, "/api/user/token", nil, user.Id)
	assert.False(t, response.Success)
	assert.NotEmpty(t, response.Message)

	var got model.User
	require.NoError(t, model.DB.First(&got, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, got.Status)
	assert.Equal(t, oldToken, got.GetAccessToken())
}

func TestGenerateAccessTokenWritesOnlyTokenForEnabledUser(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	user := &model.User{
		Username: "controller-enabled-token",
		Password: "hashed",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "premium",
	}
	require.NoError(t, model.DB.Create(user).Error)

	response := callUserRedemptionController(t, GenerateAccessToken, http.MethodGet, "/api/user/token", nil, user.Id)
	assert.True(t, response.Success)

	var got model.User
	require.NoError(t, model.DB.First(&got, user.Id).Error)
	assert.NotEmpty(t, got.GetAccessToken())
	assert.Equal(t, common.UserStatusEnabled, got.Status)
	assert.Equal(t, common.RoleAdminUser, got.Role)
	assert.Equal(t, "premium", got.Group)
}

func TestGenerateAccessTokenHandlesLookupError(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	missing := callUserRedemptionController(t, GenerateAccessToken, http.MethodGet, "/api/user/token", nil, 99999)
	assert.False(t, missing.Success)
}

func TestGenerateAccessTokenRejectsDuplicateAndUpdateErrors(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	target := &model.User{Username: "controller-duplicate-target", Password: "hashed", Status: common.UserStatusEnabled, AffCode: "duplicate-target"}
	require.NoError(t, model.DB.Create(target).Error)
	for i, length := range []int{29, 30, 31, 32} {
		token := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("A"), length*3/4))
		blocker := &model.User{
			Username:    fmt.Sprintf("controller-duplicate-blocker-%d", i),
			Password:    "hashed",
			Status:      common.UserStatusEnabled,
			AccessToken: &token,
			AffCode:     fmt.Sprintf("duplicate-blocker-%d", i),
		}
		require.NoError(t, model.DB.Create(blocker).Error)
	}
	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte("A"), 256))
	t.Cleanup(func() { crand.Reader = originalReader })
	second := callUserRedemptionController(t, GenerateAccessToken, http.MethodGet, "/api/user/token", nil, target.Id)
	assert.False(t, second.Success)
	assert.NotEmpty(t, second.Message)

	require.NoError(t, model.DB.Model(&model.User{}).Where("access_token IS NOT NULL").Update("access_token", nil).Error)
	require.NoError(t, model.DB.Exec("CREATE TRIGGER access_token_update_error BEFORE UPDATE OF access_token ON users BEGIN SELECT RAISE(ABORT, 'forced access token error'); END").Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER access_token_update_error").Error })
	response := callUserRedemptionController(t, GenerateAccessToken, http.MethodGet, "/api/user/token", nil, target.Id)
	assert.False(t, response.Success)
}

func TestGetAffCodeCreatesOnceAndReusesExistingValue(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	user := &model.User{Username: "controller-aff-code", Password: "hashed", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)

	first := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, user.Id)
	assert.True(t, first.Success)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.NotEmpty(t, stored.AffCode)

	second := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, user.Id)
	assert.True(t, second.Success)
	assert.Equal(t, stored.AffCode, second.Data)
}

func TestGetAffCodeAssignsHistoricalNullValue(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	user := &model.User{Username: "controller-aff-null", Password: "hashed", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Exec("UPDATE users SET aff_code = NULL WHERE id = ?", user.Id).Error)

	response := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, user.Id)
	assert.True(t, response.Success)
	assert.NotEmpty(t, response.Data)

	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.Equal(t, response.Data, stored.AffCode)
	assert.NotEmpty(t, stored.AffCode)
}

func TestGetAffCodeHandlesMissingAndDatabaseErrors(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	missing := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, 99999)
	assert.False(t, missing.Success)

	user := &model.User{Username: "controller-aff-error", Password: "hashed", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Exec("CREATE TRIGGER aff_code_update_error BEFORE UPDATE OF aff_code ON users BEGIN SELECT RAISE(ABORT, 'forced aff code error'); END").Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER aff_code_update_error").Error })
	response := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, user.Id)
	assert.False(t, response.Success)
}

func TestGetAffCodeUsesCodeWonByConcurrentWriter(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	user := &model.User{Username: "controller-aff-race", Password: "hashed", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	trigger := fmt.Sprintf("CREATE TRIGGER aff_code_race BEFORE UPDATE OF aff_code ON users BEGIN UPDATE users SET aff_code = 'race-code' WHERE id = OLD.id; SELECT RAISE(IGNORE); END")
	require.NoError(t, model.DB.Exec(trigger).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER aff_code_race").Error })
	response := callUserRedemptionController(t, GetAffCode, http.MethodGet, "/api/user/aff_code", nil, user.Id)
	assert.True(t, response.Success)
	assert.Equal(t, "race-code", response.Data)
}

func TestUpdateRedemptionSeparatesStatusOnlyWrite(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	redemption := &model.Redemption{
		Name:         "controller-redemption",
		Key:          "20000000000000000000000000000001",
		Status:       common.RedemptionCodeStatusUsed,
		Quota:        100,
		RedeemedTime: 12345,
		UsedUserId:   8,
	}
	require.NoError(t, model.DB.Create(redemption).Error)

	response := callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/", map[string]any{
		"id":           redemption.Id,
		"name":         "edited-name",
		"quota":        250,
		"expired_time": int64(0),
		"status":       common.RedemptionCodeStatusEnabled,
	}, 1)
	assert.True(t, response.Success)
	var got model.Redemption
	require.NoError(t, model.DB.First(&got, redemption.Id).Error)
	assert.Equal(t, "edited-name", got.Name)
	assert.Equal(t, 250, got.Quota)
	assert.Equal(t, common.RedemptionCodeStatusUsed, got.Status)
	assert.Equal(t, int64(12345), got.RedeemedTime)

	response = callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/?status_only=true", map[string]any{
		"id":     redemption.Id,
		"status": common.RedemptionCodeStatusDisabled,
	}, 1)
	assert.True(t, response.Success)
	require.NoError(t, model.DB.First(&got, redemption.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusDisabled, got.Status)
	assert.Equal(t, int64(12345), got.RedeemedTime)
}

func TestUpdateRedemptionHandlesValidationAndWriteErrors(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	invalidJSON := callRawUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/", "{", 1)
	assert.False(t, invalidJSON.Success)
	missing := callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/", map[string]any{"id": 99999}, 1)
	assert.False(t, missing.Success)

	redemption := &model.Redemption{Name: "validation-redemption", Key: "20000000000000000000000000000002", Status: common.RedemptionCodeStatusEnabled, Quota: 100}
	require.NoError(t, model.DB.Create(redemption).Error)
	expired := callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/", map[string]any{
		"id": redemption.Id, "name": "expired", "quota": 100, "expired_time": common.GetTimestamp() - 1,
	}, 1)
	assert.False(t, expired.Success)

	require.NoError(t, model.DB.Exec("CREATE TRIGGER redemption_update_error BEFORE UPDATE OF name ON redemptions BEGIN SELECT RAISE(ABORT, 'forced redemption error'); END").Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER redemption_update_error").Error })
	writeError := callUserRedemptionController(t, UpdateRedemption, http.MethodPut, "/api/redemption/", map[string]any{
		"id": redemption.Id, "name": "write-error", "quota": 100, "expired_time": int64(0),
	}, 1)
	assert.False(t, writeError.Success)
}
