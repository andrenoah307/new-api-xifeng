package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiStreamHandlerKeepsMalformedChunkValidation(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	malformed := `{"id":`
	body := strings.Join([]string{
		`data: {"id":"chunk-1","choices":[{"delta":{"content":"ok"}}]}`,
		"data: " + malformed,
		`data: {"id":"chunk-2","choices":[{"delta":{}}]}`,
		`data: [DONE]`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-model"},
		RelayMode:   relayconstant.RelayModeChatCompletions,
	}

	var expected dto.ChatCompletionsStreamResponse
	expectedErr := common.UnmarshalJsonStr(malformed, &expected)
	require.Error(t, expectedErr)

	_, handlerErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	require.NotNil(t, info.StreamStatus)
	require.Equal(t, 1, info.StreamStatus.TotalErrorCount())
	require.Len(t, info.StreamStatus.Errors, 1)
	assert.Equal(t, expectedErr.Error(), info.StreamStatus.Errors[0].Message)
	assert.Equal(t, 3, info.ReceivedResponseCount, "the malformed chunk remains a soft validation error")
}

func TestOaiStreamHandlerRejectsMissingResponseBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := OaiStreamHandler(c, &relaycommon.RelayInfo{}, &http.Response{})
	require.NotNil(t, err)
}

func TestOaiStreamHandlerExtractsAudioUsageFromSecondLastChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	oldDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = oldDebug })

	body := strings.Join([]string{
		`data: {"id":"audio-1","choices":[]}`,
		`data: {"id":"audio-2","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`,
		`data: {"id":"audio-3","choices":[]}`,
		`data: [DONE]`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "audio-test"},
		RelayMode:   relayconstant.RelayModeChatCompletions,
	}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 4, usage.PromptTokens)
	assert.Equal(t, 6, usage.CompletionTokens)
	assert.Equal(t, 10, usage.TotalTokens)
}

func TestOaiStreamHandlerForwardsOpenAIStreamAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"chat-1","choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"id":"chat-2","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "test-model"},
		RelayMode:          relayconstant.RelayModeChatCompletions,
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
	}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), "hello")
}

func TestOaiStreamHandlerReportsFormatAndFinalChunkErrors(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"chat-1","choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"id":`,
		`data: {"id":"chat-2","choices":[]}`,
		`data: [DONE]`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "test-model",
			ChannelSetting:    dto.ChannelSettings{ForceFormat: true},
		},
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
	}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	_, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, info.StreamStatus)
	assert.GreaterOrEqual(t, info.StreamStatus.TotalErrorCount(), 2)
}

func TestOaiResponsesStreamHandlerRejectsMalformedChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	c, _, resp, info := newResponsesChatTestContext(t, "data: {\"type\":\n\ndata: [DONE]\n", true)
	_, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, 1, info.StreamStatus.TotalErrorCount())
}

func TestOaiResponsesStreamHandlerExtractsUsageImageAndToolCall(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	completed := dto.ResponsesStreamResponse{
		Type: "response.completed",
		Response: &dto.OpenAIResponsesResponse{
			Usage: &dto.Usage{
				InputTokens:  11,
				OutputTokens: 13,
				TotalTokens:  24,
				InputTokensDetails: &dto.InputTokenDetails{
					CachedTokens:     2,
					CacheWriteTokens: 3,
				},
			},
			Output: []dto.ResponsesOutput{{
				Type:    dto.ResponsesOutputTypeImageGenerationCall,
				Quality: "high",
				Size:    "1024x1024",
			}},
		},
	}
	itemDone := dto.ResponsesStreamResponse{
		Type: dto.ResponsesOutputTypeItemDone,
		Item: &dto.ResponsesOutput{Type: dto.BuildInCallWebSearchCall},
	}
	completedJSON, err := common.Marshal(completed)
	require.NoError(t, err)
	itemDoneJSON, err := common.Marshal(itemDone)
	require.NoError(t, err)
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"x"}`,
		"data: " + string(completedJSON),
		"data: " + string(itemDoneJSON),
		`data: [DONE]`,
		"",
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.ResponsesUsageInfo = &relaycommon.ResponsesUsageInfo{
		BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
			dto.BuildInToolWebSearchPreview: {},
		},
	}

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.PromptTokens)
	assert.Equal(t, 13, usage.CompletionTokens)
	assert.Equal(t, 24, usage.TotalTokens)
	assert.Equal(t, 2, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
	assert.True(t, c.GetBool("image_generation_call"))
	assert.Equal(t, "high", c.GetString("image_generation_call_quality"))
	assert.Equal(t, "1024x1024", c.GetString("image_generation_call_size"))
	assert.Contains(t, recorder.Body.String(), "response.completed")
}

func TestProcessTokenDataValidatesSupportedRelayModes(t *testing.T) {
	tests := []struct {
		name      string
		relayMode int
		data      string
		wantErr   bool
	}{
		{
			name:      "chat completion",
			relayMode: relayconstant.RelayModeChatCompletions,
			data:      `{"choices":[{"delta":{"content":"ok"}}]}`,
		},
		{
			name:      "legacy completion",
			relayMode: relayconstant.RelayModeCompletions,
			data:      `{"choices":[{"text":"ok"}]}`,
		},
		{
			name:      "malformed chat completion",
			relayMode: relayconstant.RelayModeChatCompletions,
			data:      `{"choices":`,
			wantErr:   true,
		},
		{
			name:      "malformed legacy completion",
			relayMode: relayconstant.RelayModeCompletions,
			data:      `{"choices":`,
			wantErr:   true,
		},
		{
			name:      "unknown mode skips token accounting",
			relayMode: relayconstant.RelayModeUnknown,
			data:      `not-json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processTokenData(tt.relayMode, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOaiResponsesStreamRuneCounterMatchesWholeText(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "empty stream", chunks: nil},
		{name: "single rune split", chunks: []string{"a", "b"}},
		{name: "CJK", chunks: []string{"你", "好"}},
		{name: "emoji", chunks: []string{"😀", "🎉"}},
		{name: "odd rune count", chunks: []string{"a", "b", "c", "d", "e"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := make([]string, 0, len(tt.chunks)+1)
			for _, chunk := range tt.chunks {
				encoded, err := common.Marshal(dto.ResponsesStreamResponse{
					Type:  "response.output_text.delta",
					Delta: chunk,
				})
				require.NoError(t, err)
				lines = append(lines, "data: "+string(encoded))
			}
			lines = append(lines, "data: [DONE]", "")

			c, _, resp, info := newResponsesChatTestContext(t, strings.Join(lines, "\n"), true)
			usage, err := OaiResponsesStreamHandler(c, info, resp)
			require.Nil(t, err)
			require.NotNil(t, usage)
			want := service.CountTextToken(strings.Join(tt.chunks, ""), info.UpstreamModelName)
			assert.Equal(t, want, usage.CompletionTokens)
		})
	}
}
