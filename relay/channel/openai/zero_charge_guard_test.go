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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiStreamHandlerPromptOnlyEmptyStreamZeroCharges(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	usage := `{"prompt_tokens":1668374,"completion_tokens":0,"total_tokens":1668374}`
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-empty","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-empty","choices":[{"delta":{},"finish_reason":"stop"}],"usage":` + usage + `}`,
		`data: [DONE]`,
		"",
	}, "\n")

	info := &relaycommon.RelayInfo{
		RelayFormat:        types.RelayFormatOpenAI,
		RelayMode:          relayconstant.RelayModeChatCompletions,
		IsStream:           true,
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		ShouldIncludeUsage: true,
	}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	got, err := OaiStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, got)
	assert.Equal(t, dto.Usage{}, *got)
	assert.True(t, info.ZeroChargeGuardTriggered)
	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, relaycommon.ZeroChargeReasonEmptyOutput, info.ZeroChargeGuardSnapshot.Reason)
	assert.Equal(t, 1668374, info.ZeroChargeGuardSnapshot.PromptTokens)
	assert.Contains(t, recorder.Body.String(), `"prompt_tokens":0`)
}

func TestOaiStreamHandlerCacheOnlyUsageRequiresDeliverableOutput(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name          string
		firstChunk    string
		wantGuard     bool
		wantCacheRead int
	}{
		{
			name:          "cache-only usage with text output is billable",
			firstChunk:    `{"id":"chat-cache","choices":[{"delta":{"content":"hello"}}]}`,
			wantCacheRead: 17,
		},
		{
			name:       "cache-only usage without output is zero charged",
			firstChunk: `{"id":"chat-cache","choices":[{"delta":{"role":"assistant"}}]}`,
			wantGuard:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Join([]string{
				"data: " + tt.firstChunk,
				`data: {"id":"chat-cache","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"prompt_tokens_details":{"cached_tokens":17}}}`,
				`data: [DONE]`,
				"",
			}, "\n")

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat:        types.RelayFormatOpenAI,
				RelayMode:          relayconstant.RelayModeChatCompletions,
				ShouldIncludeUsage: true,
				ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-cache"},
			}
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

			got, err := OaiStreamHandler(c, info, resp)
			require.Nil(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantGuard, info.ZeroChargeGuardTriggered)
			if tt.wantGuard {
				assert.Equal(t, dto.Usage{}, *got)
				require.NotNil(t, info.ZeroChargeGuardSnapshot)
				assert.Equal(t, 17, info.ZeroChargeGuardSnapshot.CacheReadTokens)
			} else {
				assert.Equal(t, tt.wantCacheRead, got.PromptTokensDetails.CachedTokens)
				assert.True(t, info.HasDeliverableOutput)
				assert.NotEqual(t, dto.Usage{}, *got, "cache-bearing usage must reach settlement unchanged")
				assert.False(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
			}
		})
	}
}

func TestOaiStreamHandlerDoesNotDoubleCountFinalChatChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"chat-final","choices":[{"delta":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":0,"total_tokens":10}}`,
		`data: [DONE]`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	_, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	assert.True(t, info.HasDeliverableOutput)
	assert.Equal(t, 5, info.OutputRuneCount)
}

func TestHandleStreamFormatMarksDecodedChatOutput(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{
				RelayFormat: format,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
				ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{
					LastMessagesType: relaycommon.LastMessageTypeNone,
				},
			}
			err := HandleStreamFormat(c, info, `{"id":"chat-format","choices":[{"delta":{"content":"hello"}}]}`, false, false)
			require.NoError(t, err)
			assert.True(t, info.HasDeliverableOutput)
			assert.Equal(t, 5, info.OutputRuneCount)
		})
	}
}

func TestHandleLastResponseMarksDeferredConvertedChunk(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	last := `{"id":"chat-final","choices":[{"delta":{"content":"hello"}}]}`
	var responseID string
	var created int64
	var fingerprint string
	var model string
	usage := &dto.Usage{}
	containUsage := false
	shouldSend := true

	err := handleLastResponse(last, &responseID, &created, &fingerprint, &model, &usage,
		&containUsage, info, &shouldSend)
	require.NoError(t, err)
	assert.True(t, info.HasDeliverableOutput)
	assert.Equal(t, 5, info.OutputRuneCount)
}

func TestOpenaiHandlerPromptOnlyOutputQuadrants(t *testing.T) {
	tests := []struct {
		name       string
		choice     string
		wantGuard  bool
		wantPrompt int
	}{
		{
			name:      "empty output",
			choice:    `{"message":{"role":"assistant","content":""},"finish_reason":"stop"}`,
			wantGuard: true,
		},
		{
			name:       "text output",
			choice:     `{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}`,
			wantPrompt: 100,
		},
		{
			name:       "reasoning output",
			choice:     `{"message":{"role":"assistant","content":"","reasoning_content":"think"},"finish_reason":"stop"}`,
			wantPrompt: 100,
		},
		{
			name:       "tool output",
			choice:     `{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{}}]},"finish_reason":"tool_calls"}`,
			wantPrompt: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			body := `{"choices":[` + tt.choice + `],"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}`
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

			got, err := OpenaiHandler(c, info, resp)
			require.Nil(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantGuard, info.ZeroChargeGuardTriggered)
			if tt.wantGuard {
				assert.Zero(t, got.PromptTokens)
			} else {
				assert.Equal(t, tt.wantPrompt, got.PromptTokens)
			}
		})
	}
}

func TestMarkChatCompletionsOutputDoesNotCountControlChunks(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	role := &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}}}}
	text := &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: common.GetPointer("x")}}}}
	relaycommon.MarkChatCompletionsOutput(info, role)
	assert.False(t, info.HasDeliverableOutput)
	relaycommon.MarkChatCompletionsOutput(info, text)
	assert.True(t, info.HasDeliverableOutput)
	assert.Equal(t, 1, info.OutputRuneCount)
}

func TestHandleFinalResponseUsesSettledUsageForConvertedFormats(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{
				RelayFormat:       format,
				ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
				ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{},
			}
			last := `{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1601,"completion_tokens":0,"total_tokens":1601}}`
			settled := &dto.Usage{}
			HandleFinalResponse(c, info, last, "id", 1, "gpt-test", "", settled, true)
			assert.NotContains(t, recorder.Body.String(), `"prompt_tokens":1601`)
		})
	}
}

func TestHandleFinalResponseClaudeDoesNotRebuildStaleBillingUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		SendResponseCount: 1,
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "claude-test"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{
			LastMessagesType: relaycommon.LastMessageTypeNone,
		},
	}

	staleBilling := dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
		InputTokens:              1601,
		CacheCreationInputTokens: 553250,
		OutputTokens:             0,
	})
	lastResponse := dto.ChatCompletionsStreamResponse{
		Id:    "chat-stale",
		Model: "claude-test",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
			FinishReason: common.GetPointer("stop"),
		}},
		Usage: &dto.Usage{
			PromptTokens: 1601,
			BillingUsage: staleBilling,
		},
	}
	lastBytes, err := common.Marshal(lastResponse)
	require.NoError(t, err)

	settled := &dto.Usage{
		PromptTokens: 1601,
		BillingUsage: staleBilling,
	}
	relaycommon.CloseoutZeroCharge(info, settled, relaycommon.ZeroChargeReasonEmptyOutput)
	HandleFinalResponse(c, info, string(lastBytes), "chat-stale", 1, "claude-test", "", settled, true)

	require.NotNil(t, info.ClaudeConvertInfo.Usage)
	assert.Nil(t, info.ClaudeConvertInfo.Usage.BillingUsage)
	assert.NotContains(t, recorder.Body.String(), "553250")
}
