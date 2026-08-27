package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newZeroChargeResponseContext(t *testing.T, format types.RelayFormat) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat:        format,
		ShouldIncludeUsage: true,
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	return c, recorder, info
}

func newJSONResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestResponsesPromptOnlyEmptyOutputZeroChargesAtEveryNonStreamEntryPoint(t *testing.T) {
	responseBody := []byte(`{"id":"resp_1","object":"response","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":0,"total_tokens":12}}`)
	chatBody := []byte(`{"id":"chat_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":0,"total_tokens":12}}`)
	tests := []struct {
		name   string
		format types.RelayFormat
		body   []byte
		call   func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "direct responses", format: types.RelayFormatOpenAIResponses, body: responseBody, call: OaiResponsesHandler},
		{name: "responses to chat", format: types.RelayFormatOpenAI, body: responseBody, call: OaiResponsesToChatHandler},
		{name: "chat to responses", format: types.RelayFormatOpenAIResponses, body: chatBody, call: OaiChatToResponsesHandler},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, info := newZeroChargeResponseContext(t, tt.format)
			usage, err := tt.call(c, info, newJSONResponse(tt.body))
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Equal(t, 0, usage.PromptTokens)
			assert.Equal(t, 0, usage.CompletionTokens)
			assert.True(t, info.ZeroChargeGuardTriggered)
		})
	}
}

func TestResponsesZeroChargeGuardLeavesExcludedProviderContractsUntouched(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeXai, constant.ChannelCloudflare} {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType}}
		usage := &dto.Usage{PromptTokens: 12}
		got := finalizeResponsesUsage(info, usage, false, false)
		require.Same(t, usage, got)
		assert.Equal(t, 12, got.PromptTokens)
		assert.False(t, info.ZeroChargeGuardTriggered)
	}
}

func TestResponsesTerminalOutputKindsPreventEmptyGuard(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "message", output: `[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]`},
		{name: "reasoning", output: `[{"type":"reasoning","content":[{"type":"summary_text","text":"thinking"}]}]`},
		{name: "tool", output: `[{"type":"function_call","name":"lookup","arguments":"{}"}]`},
		{name: "image", output: `[{"type":"image_generation_call","quality":"high","size":"1024x1024"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"id":"resp_1","object":"response","status":"completed","output":` + tt.output + `,"usage":{"input_tokens":12,"output_tokens":0,"total_tokens":12}}`)
			c, _, info := newZeroChargeResponseContext(t, types.RelayFormatOpenAIResponses)
			usage, err := OaiResponsesHandler(c, info, newJSONResponse(body))
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.False(t, info.ZeroChargeGuardTriggered)
			assert.Equal(t, 12, usage.PromptTokens)
		})
	}
}

func TestResponsesPromptOnlyEmptyOutputZeroChargesAcrossStreamWrappers(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	responsesStream := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":12,\"output_tokens\":0,\"total_tokens\":12}}}\ndata: [DONE]\n"
	chatStream := "data: {\"id\":\"chat_1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":0,\"total_tokens\":12}}\ndata: [DONE]\n"
	tests := []struct {
		name string
		body string
		call func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "direct responses", body: responsesStream, call: OaiResponsesStreamHandler},
		{name: "responses to chat", body: responsesStream, call: OaiResponsesToChatStreamHandler},
		{name: "chat to responses", body: chatStream, call: OaiChatToResponsesStreamHandler},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, info := newZeroChargeResponseContext(t, types.RelayFormatOpenAI)
			info.IsStream = true
			usage, err := tt.call(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(tt.body))})
			require.Nil(t, err)
			require.NotNil(t, usage)
			assert.Zero(t, usage.PromptTokens)
			assert.Zero(t, usage.CompletionTokens)
			assert.True(t, info.ZeroChargeGuardTriggered)
			assert.Contains(t, recorder.Body.String(), `"input_tokens":0`)
		})
	}
}
