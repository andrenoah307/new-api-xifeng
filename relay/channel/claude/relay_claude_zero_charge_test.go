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
