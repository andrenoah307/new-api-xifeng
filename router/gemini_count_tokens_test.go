package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiCountTokensIsRejectedBeforeAuthentication(t *testing.T) {
	// An unsupported action must never reach channel selection or accounting.
	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = nil, nil
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB })
	engine := gin.New()
	SetRelayRouter(engine)
	for _, query := range []string{"", "?alt=sse"} {
		t.Run(query, func(t *testing.T) {
			body := `{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]}]}`
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:countTokens"+query, strings.NewReader(body))
			require.NotPanics(t, func() { engine.ServeHTTP(recorder, request) })
			require.Equal(t, http.StatusNotFound, recorder.Code)
			var payload struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Equal(t, "invalid_request_error", payload.Error.Type)
		})
	}
	for _, action := range []string{"generateContent", "streamGenerateContent", "embedContent"} {
		t.Run(action, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:"+action, nil)
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
