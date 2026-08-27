package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleStreamFinalResponseZeroChargesEmptyOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-7-sonnet"},
	}
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokens: 2934537,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 100,
			},
			ClaudeCacheCreation5mTokens: 50,
		},
		Done: true,
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Zero(t, claudeInfo.Usage.PromptTokens, "PromptTokens 应当被清零")
	require.Zero(t, claudeInfo.Usage.CompletionTokens, "CompletionTokens 应当保持 0")
	require.Zero(t, claudeInfo.Usage.TotalTokens, "TotalTokens 应当被清零")
	require.Zero(t, claudeInfo.Usage.PromptTokensDetails.CachedTokens, "CachedTokens 应当被清零")
	require.Zero(t, claudeInfo.Usage.ClaudeCacheCreation5mTokens, "ClaudeCacheCreation5mTokens 应当被清零")
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), "应当标记 LocalCountTokens")
}

func TestHandleStreamFinalResponseKeepsUsageWithResponseText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-7-sonnet"},
	}
	claudeInfo := &ClaudeResponseInfo{
		Usage:                 &dto.Usage{PromptTokens: 100},
		Done:                  true,
		ResponseTextRuneCount: utf8.RuneCountInString("hello world from upstream"),
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Equal(t, 100, claudeInfo.Usage.PromptTokens, "有响应文本时 PromptTokens 应当保留")
	require.Greater(t, claudeInfo.Usage.CompletionTokens, 0, "fallback 估算应当填出非零 CompletionTokens")
}

func TestHandleStreamFinalResponseOpenAIEmitsUsageAndDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "claude-3-7-sonnet"},
	}
	claudeInfo := &ClaudeResponseInfo{
		ResponseId: "msg_test",
		Created:    1710000000,
		Model:      "claude-3-7-sonnet",
		Usage: &dto.Usage{
			PromptTokens:     5,
			CompletionTokens: 7,
			TotalTokens:      12,
		},
		Done: true,
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	assert.Contains(t, w.Body.String(), `"prompt_tokens":5`)
	assert.Contains(t, w.Body.String(), `data: [DONE]`)
}

func TestHandleStreamFinalResponse_ZeroChargesClientGoneNoUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "glm-5.2-fast"},
	}
	info.SetEstimatePromptTokens(737400)
	claudeInfo := &ClaudeResponseInfo{
		Usage:                 &dto.Usage{},
		Done:                  false,
		ResponseTextRuneCount: utf8.RuneCountInString("partial output text"),
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Zero(t, claudeInfo.Usage.PromptTokens, "断流且上游 usage 缺失时 PromptTokens 应当清零")
	require.Zero(t, claudeInfo.Usage.CompletionTokens, "断流且上游 usage 缺失时 CompletionTokens 应当清零")
	require.Zero(t, claudeInfo.Usage.TotalTokens, "断流且上游 usage 缺失时 TotalTokens 应当清零")
	require.Zero(t, claudeInfo.Usage.PromptTokensDetails, "断流且上游 usage 缺失时 PromptTokensDetails 应当清零")
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), "应当标记 LocalCountTokens")
}

func TestHandleStreamFinalResponse_PreservesRealPromptOnClientGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "glm-5.2-fast"},
	}
	info.SetEstimatePromptTokens(737400)
	claudeInfo := &ClaudeResponseInfo{
		Usage:                 &dto.Usage{PromptTokens: 170000},
		Done:                  false,
		ResponseTextRuneCount: utf8.RuneCountInString("partial output text"),
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Equal(t, 170000, claudeInfo.Usage.PromptTokens, "真实 PromptTokens 不应被估算覆盖或归零")
	require.Greater(t, claudeInfo.Usage.CompletionTokens, 0, "应当保留 completion fallback")
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), "不应标记 LocalCountTokens")
}

func TestHandleStreamFinalResponse_CacheOnlyNotZeroed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "glm-5.2-fast"},
	}
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 50000,
			},
		},
		Done:                  true,
		ResponseTextRuneCount: utf8.RuneCountInString("partial output text"),
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Equal(t, 50000, claudeInfo.Usage.PromptTokensDetails.CachedTokens, "全缓存命中不应被误清零")
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), "不应标记 LocalCountTokens")
}

func TestClaudeResponseHasContentDetectsText(t *testing.T) {
	text := "hello"
	resp := &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Type: "text", Text: &text}}}
	require.True(t, claudeResponseHasContent(resp))
}

func TestClaudeResponseHasContentDetectsToolUse(t *testing.T) {
	resp := &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Type: "tool_use"}}}
	require.True(t, claudeResponseHasContent(resp))
}

func TestClaudeResponseHasContentRejectsEmpty(t *testing.T) {
	require.False(t, claudeResponseHasContent(nil))
	require.False(t, claudeResponseHasContent(&dto.ClaudeResponse{}))
	emptyText := ""
	require.False(t, claudeResponseHasContent(&dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Type: "text", Text: &emptyText}}}))
}

func TestClaudeNonStreamZeroGuardClearsResidualBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-test"},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	response := &dto.ClaudeResponse{
		Type: "message",
		Usage: &dto.ClaudeUsage{
			InputTokens:  1601,
			OutputTokens: 0,
			BillingUsage: &dto.BillingUsage{
				Source:      dto.BillingUsageSourceClaudeMessages,
				Semantic:    dto.BillingUsageSemanticAnthropic,
				ClaudeUsage: &dto.ClaudeUsage{InputTokens: 1601, OutputTokens: 553250},
			},
		},
	}
	body, err := common.Marshal(response)
	require.NoError(t, err)
	httpResp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}

	require.Nil(t, HandleClaudeResponseData(c, info, claudeInfo, httpResp, body))
	assert.True(t, info.ZeroChargeGuardTriggered)
	assert.Equal(t, dto.Usage{}, *claudeInfo.Usage)
	assert.Nil(t, claudeInfo.Usage.BillingUsage)
	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, 553250, info.ZeroChargeGuardSnapshot.CompletionTokens)
}

func TestClaudeNonStreamToolOnlyOutputPreservesPromptUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-test"},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	response := &dto.ClaudeResponse{
		Type:    "message",
		Content: []dto.ClaudeMediaMessage{{Type: "tool_use"}},
		Usage:   &dto.ClaudeUsage{InputTokens: 1601, OutputTokens: 0},
	}
	body, err := common.Marshal(response)
	require.NoError(t, err)
	httpResp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}

	require.Nil(t, HandleClaudeResponseData(c, info, claudeInfo, httpResp, body))
	assert.False(t, info.ZeroChargeGuardTriggered)
	assert.Equal(t, 1601, claudeInfo.Usage.PromptTokens)
}

func TestClaudeMissingUsageWithTextStillZeroCharges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-test"},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	text := "visible output"
	response := &dto.ClaudeResponse{
		Type:    "message",
		Content: []dto.ClaudeMediaMessage{{Type: "text", Text: &text}},
	}
	body, err := common.Marshal(response)
	require.NoError(t, err)
	httpResp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}

	require.Nil(t, HandleClaudeResponseData(c, info, claudeInfo, httpResp, body))
	assert.True(t, info.ZeroChargeGuardTriggered)
	assert.Equal(t, dto.Usage{}, *claudeInfo.Usage)
	assert.Equal(t, relaycommon.ZeroChargeReasonUsageMissing, info.ZeroChargeGuardSnapshot.Reason)
}
