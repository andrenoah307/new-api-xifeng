package controller

import (
	"bytes"
	crand "crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type affCodeHTTPResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Data    string `json:"data"`
}

func callAffCodeHTTPHandler(t *testing.T, handler gin.HandlerFunc, method string, path string, body any, userID int) (int, affCodeHTTPResponse, string) {
	t.Helper()
	var requestBody *bytes.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, requestBody)
	ctx.Request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		ctx.Set("id", userID)
	}
	handler(ctx)

	var response affCodeHTTPResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder.Code, response, recorder.Body.String()
}

func configureAffCodeRegistrationTest(t *testing.T) {
	t.Helper()
	originalRegisterEnabled := common.RegisterEnabled
	originalPasswordRegisterEnabled := common.PasswordRegisterEnabled
	originalEmailVerificationEnabled := common.EmailVerificationEnabled
	originalInvitationCodeEnabled := common.InvitationCodeEnabled
	originalGenerateDefaultToken := constant.GenerateDefaultToken
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.InvitationCodeEnabled = false
	constant.GenerateDefaultToken = false
	t.Cleanup(func() {
		common.RegisterEnabled = originalRegisterEnabled
		common.PasswordRegisterEnabled = originalPasswordRegisterEnabled
		common.EmailVerificationEnabled = originalEmailVerificationEnabled
		common.InvitationCodeEnabled = originalInvitationCodeEnabled
		constant.GenerateDefaultToken = originalGenerateDefaultToken
	})
}

func assertAffCodeErrorIsRedacted(t *testing.T, status int, response affCodeHTTPResponse, rawBody string, code string, secrets ...string) {
	t.Helper()
	assert.Equal(t, http.StatusBadRequest, status)
	assert.False(t, response.Success)
	assert.Equal(t, code, response.Code)
	assert.NotEmpty(t, response.Message)
	for _, secret := range secrets {
		assert.NotContains(t, rawBody, secret)
	}
}

func TestRegisterReturnsCodedBadRequestWhenAffCodeRetriesAreExhausted(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	configureAffCodeRegistrationTest(t)
	require.NoError(t, model.DB.Create(&model.User{Username: "register-aff-blocker", Password: "hashed", AffCode: "22222222"}).Error)

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte{0}, 16+5*8))
	t.Cleanup(func() { crand.Reader = originalReader })

	status, response, rawBody := callAffCodeHTTPHandler(t, Register, http.MethodPost, "/api/user/register", map[string]any{
		"username": "new-aff-user",
		"password": "password123",
	}, 0)
	assertAffCodeErrorIsRedacted(t, status, response, rawBody, i18n.MsgUserAffCodeGenerateFailed,
		"users", "idx_users_aff_code", "22222222", "UNIQUE constraint failed")
	assert.Equal(t, "Failed to generate affiliate code. Please try again.", response.Message)
}

func TestRegisterRedactsUnexpectedInsertError(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	configureAffCodeRegistrationTest(t)
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER register_insert_error BEFORE INSERT ON users BEGIN SELECT RAISE(ABORT, 'users idx_users_aff_code SECRET22 raw-driver-error'); END`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER register_insert_error").Error })

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte{0}, 16+8))
	t.Cleanup(func() { crand.Reader = originalReader })

	status, response, rawBody := callAffCodeHTTPHandler(t, Register, http.MethodPost, "/api/user/register", map[string]any{
		"username": "insert-error-user",
		"password": "password123",
	}, 0)
	assertAffCodeErrorIsRedacted(t, status, response, rawBody, i18n.MsgUserRegisterFailed,
		"users", "idx_users_aff_code", "22222222", "SECRET22", "raw-driver-error")
	assert.Equal(t, "User registration failed or user ID retrieval failed", response.Message)
}

func TestGetAffCodeRetriesCollisionAndReturnsNewCode(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Username: "get-aff-blocker", Password: "hashed", AffCode: "22222222"}).Error)
	target := &model.User{Username: "get-aff-target", Password: "hashed"}
	require.NoError(t, model.DB.Create(target).Error)

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(append(bytes.Repeat([]byte{0}, 8), bytes.Repeat([]byte{1}, 8)...))
	t.Cleanup(func() { crand.Reader = originalReader })

	status, response, _ := callAffCodeHTTPHandler(t, GetAffCode, http.MethodGet, "/api/user/aff", nil, target.Id)
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, response.Success)
	assert.Equal(t, "33333333", response.Data)

	var stored model.User
	require.NoError(t, model.DB.First(&stored, target.Id).Error)
	assert.Equal(t, "33333333", stored.AffCode)
}

func TestGetAffCodePreservesHistoricalFourCharacterCode(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	user := &model.User{Username: "historical-aff-code", Password: "hashed", AffCode: "aB3x"}
	require.NoError(t, model.DB.Create(user).Error)

	status, response, _ := callAffCodeHTTPHandler(t, GetAffCode, http.MethodGet, "/api/user/aff", nil, user.Id)
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, response.Success)
	assert.Equal(t, "aB3x", response.Data)

	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.Equal(t, "aB3x", stored.AffCode)
}

func TestGetAffCodeReturnsCodedBadRequestAfterFiveCollisions(t *testing.T) {
	setupUserRedemptionControllerDB(t)
	require.NoError(t, model.DB.Create(&model.User{Username: "get-aff-exhaust-blocker", Password: "hashed", AffCode: "22222222"}).Error)
	target := &model.User{Username: "get-aff-exhaust-target", Password: "hashed"}
	require.NoError(t, model.DB.Create(target).Error)

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte{0}, 5*8))
	t.Cleanup(func() { crand.Reader = originalReader })

	status, response, rawBody := callAffCodeHTTPHandler(t, GetAffCode, http.MethodGet, "/api/user/aff", nil, target.Id)
	assertAffCodeErrorIsRedacted(t, status, response, rawBody, i18n.MsgUserAffCodeGenerateFailed,
		"users", "idx_users_aff_code", "22222222", "UNIQUE constraint failed")
	assert.Equal(t, "Failed to generate affiliate code. Please try again.", response.Message)
}

func TestGetAffCodeRedactsLookupUpdateAndReadbackErrors(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) int
		secrets []string
	}{
		{
			name: "initial lookup",
			prepare: func(t *testing.T) int {
				return 99999
			},
			secrets: []string{"record not found"},
		},
		{
			name: "update",
			prepare: func(t *testing.T) int {
				user := &model.User{Username: "get-aff-update-error", Password: "hashed"}
				require.NoError(t, model.DB.Create(user).Error)
				require.NoError(t, model.DB.Exec(`CREATE TRIGGER aff_code_update_secret BEFORE UPDATE OF aff_code ON users BEGIN SELECT RAISE(ABORT, 'users idx_users_aff_code SECRET22 raw-driver-error'); END`).Error)
				t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER aff_code_update_secret").Error })
				return user.Id
			},
			secrets: []string{"users", "idx_users_aff_code", "SECRET22", "raw-driver-error"},
		},
		{
			name: "readback",
			prepare: func(t *testing.T) int {
				user := &model.User{Username: "get-aff-readback-error", Password: "hashed"}
				require.NoError(t, model.DB.Create(user).Error)
				require.NoError(t, model.DB.Exec(`CREATE TRIGGER aff_code_readback_error BEFORE UPDATE OF aff_code ON users BEGIN DELETE FROM users WHERE id = OLD.id; SELECT RAISE(IGNORE); END`).Error)
				t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER aff_code_readback_error").Error })
				return user.Id
			},
			secrets: []string{"record not found"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupUserRedemptionControllerDB(t)
			userID := test.prepare(t)
			originalReader := crand.Reader
			crand.Reader = bytes.NewReader(bytes.Repeat([]byte{0}, 8))
			t.Cleanup(func() { crand.Reader = originalReader })

			status, response, rawBody := callAffCodeHTTPHandler(t, GetAffCode, http.MethodGet, "/api/user/aff", nil, userID)
			assertAffCodeErrorIsRedacted(t, status, response, rawBody, i18n.MsgUserAffCodeGenerateFailed, test.secrets...)
			assert.False(t, strings.Contains(response.Message, "users"))
		})
	}
}
