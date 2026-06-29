package claude

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
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
		Done:         true,
		ResponseText: strings.Builder{},
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
	rt := strings.Builder{}
	rt.WriteString("hello world from upstream")
	claudeInfo := &ClaudeResponseInfo{
		Usage:        &dto.Usage{PromptTokens: 100},
		Done:         true,
		ResponseText: rt,
	}

	HandleStreamFinalResponse(ctx, info, claudeInfo)

	require.Equal(t, 100, claudeInfo.Usage.PromptTokens, "有响应文本时 PromptTokens 应当保留")
	require.Greater(t, claudeInfo.Usage.CompletionTokens, 0, "fallback 估算应当填出非零 CompletionTokens")
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
	rt := strings.Builder{}
	rt.WriteString("partial output text")
	claudeInfo := &ClaudeResponseInfo{
		Usage:        &dto.Usage{},
		Done:         false,
		ResponseText: rt,
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
	rt := strings.Builder{}
	rt.WriteString("partial output text")
	claudeInfo := &ClaudeResponseInfo{
		Usage:        &dto.Usage{PromptTokens: 170000},
		Done:         false,
		ResponseText: rt,
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
	rt := strings.Builder{}
	rt.WriteString("partial output text")
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens: 50000,
			},
		},
		Done:         true,
		ResponseText: rt,
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
