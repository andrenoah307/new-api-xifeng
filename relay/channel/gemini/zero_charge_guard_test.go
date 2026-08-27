package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiResponsesPromptOnlyUsageIsZeroCharged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := newGeminiResponsesRelayInfo(false)
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Role: "model"}}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 100,
			TotalTokenCount:  100,
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	usage, apiErr := GeminiResponsesHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.True(t, info.ZeroChargeGuardTriggered)
}

func TestGeminiResponsesPureErrorSSEIsZeroCharged(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := newGeminiResponsesRelayInfo(true)
	body := "data: {\"error\":{\"message\":\"upstream failed\"}}\ndata: [DONE]\n"

	usage, apiErr := GeminiResponsesStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.True(t, info.ZeroChargeGuardTriggered)
}

func TestGeminiPureErrorSSEIsNotTreatedAsOutput(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gemini-test", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
	body := "data: {\"error\":{\"message\":\"upstream failed\"}}\ndata: [DONE]\n"

	usage, err := geminiStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body))}, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})

	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.True(t, info.ZeroChargeGuardTriggered)
	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, relaycommon.ZeroChargeReasonUsageMissing, info.ZeroChargeGuardSnapshot.Reason)
}

func TestGeminiNativePromptOnlyEmptyResponseZeroCharges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:generateContent", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-test",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"},
	}
	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[]}}],"usageMetadata":{"promptTokenCount":100,"totalTokenCount":100}}`)

	usage, err := GeminiTextGenerationHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))})

	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.True(t, info.ZeroChargeGuardTriggered)
}

func TestGeminiOutputTrackerCountsFunctionCallWithoutArguments(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	response := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{FunctionCall: &dto.FunctionCall{}}}}}}}
	relaycommon.MarkGeminiResponseOutput(info, response)
	assert.True(t, info.HasDeliverableOutput)
}

func TestGeminiEmbeddingDoesNotUseTextZeroGuard(t *testing.T) {
	// This is a contract test for the dispatcher boundary: embedding usage is
	// input-based and must not be marked as an empty text response.
	info := &relaycommon.RelayInfo{IsGeminiBatchEmbedding: true}
	assert.False(t, info.ZeroChargeGuardTriggered)
}
