package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiErrorI18nWithStatusReturnsMachineReadableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTranslateMessage := TranslateMessage
	TranslateMessage = func(_ *gin.Context, _ string, _ ...map[string]any) string {
		return "Safe translated message"
	}
	t.Cleanup(func() { TranslateMessage = originalTranslateMessage })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ApiErrorI18nWithStatus(ctx, http.StatusBadRequest, "user.aff_code_generate_failed")

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	require.NoError(t, Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "Safe translated message", response.Message)
	assert.Equal(t, "user.aff_code_generate_failed", response.Code)
}
