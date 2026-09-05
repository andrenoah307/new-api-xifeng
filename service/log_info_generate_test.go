package service

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoCarriesClaudeToolResultMediaMetadata(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta:                &relaycommon.ChannelMeta{},
		ToolResultImageCount:       2,
		ToolResultImageBase64Chars: 1234,
		ToolResultMediaTypes:       []string{"image/png", "application/pdf", "image/png"},
		ToolResultMediaFallback:    true,
	}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, 0)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, adminInfo["tool_result_image_count"])
	assert.Equal(t, 1234, adminInfo["tool_result_image_base64_chars"])
	assert.Equal(t, []string{"application/pdf", "image/png"}, adminInfo["tool_result_media_types"])
	assert.Equal(t, true, adminInfo["tool_result_media_fallback"])
}

func TestGenerateTextOtherInfoOmitsZeroClaudeToolResultMediaMetadata(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, 0)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, adminInfo, "tool_result_image_count")
	assert.NotContains(t, adminInfo, "tool_result_image_base64_chars")
	assert.NotContains(t, adminInfo, "tool_result_media_types")
	assert.NotContains(t, adminInfo, "tool_result_media_fallback")
}

func TestGenerateTextOtherInfoCarriesThinkingRequestFields(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	budget := 0
	info := &relaycommon.RelayInfo{
		ChannelMeta:    &relaycommon.ChannelMeta{},
		ThinkingBudget: &budget,
		ThinkingType:   "disabled",
	}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, 0)
	assert.Equal(t, 0, other["thinking_budget"])
	assert.Equal(t, "disabled", other["thinking_type"])
}
