package claude

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPatchClaudeMessageDeltaUsageDataPreserveUnknownFields(t *testing.T) {
	originalData := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":53},"vendor_meta":{"trace_id":"trace_001"}}`
	usage := &dto.ClaudeUsage{
		InputTokens:              100,
		CacheReadInputTokens:     30,
		CacheCreationInputTokens: 50,
	}

	patchedData := patchClaudeMessageDeltaUsageData(originalData, usage)

	require.Equal(t, "message_delta", gjson.Get(patchedData, "type").String())
	require.Equal(t, "end_turn", gjson.Get(patchedData, "delta.stop_reason").String())
	require.Equal(t, "trace_001", gjson.Get(patchedData, "vendor_meta.trace_id").String())
	require.EqualValues(t, 53, gjson.Get(patchedData, "usage.output_tokens").Int())
	require.EqualValues(t, 100, gjson.Get(patchedData, "usage.input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(patchedData, "usage.cache_read_input_tokens").Int())
	require.EqualValues(t, 50, gjson.Get(patchedData, "usage.cache_creation_input_tokens").Int())
}

func TestPatchClaudeMessageDeltaUsageDataZeroValueChecks(t *testing.T) {
	originalData := `{"type":"message_delta","usage":{"output_tokens":53,"input_tokens":9,"cache_read_input_tokens":0}}`
	usage := &dto.ClaudeUsage{
		InputTokens:              100,
		CacheReadInputTokens:     30,
		CacheCreationInputTokens: 0,
	}

	patchedData := patchClaudeMessageDeltaUsageData(originalData, usage)

	require.EqualValues(t, 9, gjson.Get(patchedData, "usage.input_tokens").Int())
	require.EqualValues(t, 30, gjson.Get(patchedData, "usage.cache_read_input_tokens").Int())
	assert.False(t, gjson.Get(patchedData, "usage.cache_creation_input_tokens").Exists())
}

func TestShouldSkipClaudeMessageDeltaUsagePatch(t *testing.T) {
	originGlobalPassThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originGlobalPassThrough
	})

	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true
	assert.True(t, shouldSkipClaudeMessageDeltaUsagePatch(&relaycommon.RelayInfo{}))

	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	assert.True(t, shouldSkipClaudeMessageDeltaUsagePatch(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true}},
	}))
	assert.False(t, shouldSkipClaudeMessageDeltaUsagePatch(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: false}},
	}))
}

func TestBuildMessageDeltaPatchUsage(t *testing.T) {
	t.Run("merge missing fields from claudeInfo", func(t *testing.T) {
		claudeResponse := &dto.ClaudeResponse{Usage: &dto.ClaudeUsage{OutputTokens: 53}}
		claudeInfo := &ClaudeResponseInfo{
			Usage: &dto.Usage{
				PromptTokens: 100,
				PromptTokensDetails: dto.InputTokenDetails{
					CachedTokens:         30,
					CachedCreationTokens: 50,
				},
				ClaudeCacheCreation5mTokens: 10,
				ClaudeCacheCreation1hTokens: 20,
			},
		}

		usage := buildMessageDeltaPatchUsage(claudeResponse, claudeInfo)
		require.NotNil(t, usage)
		require.EqualValues(t, 100, usage.InputTokens)
		require.EqualValues(t, 30, usage.CacheReadInputTokens)
		require.EqualValues(t, 50, usage.CacheCreationInputTokens)
		require.EqualValues(t, 53, usage.OutputTokens)
		require.NotNil(t, usage.CacheCreation)
		require.EqualValues(t, 30, usage.CacheCreation.Ephemeral5mInputTokens)
		require.EqualValues(t, 20, usage.CacheCreation.Ephemeral1hInputTokens)
	})

	t.Run("keep upstream non-zero values", func(t *testing.T) {
		claudeResponse := &dto.ClaudeResponse{Usage: &dto.ClaudeUsage{
			InputTokens:              9,
			CacheReadInputTokens:     7,
			CacheCreationInputTokens: 6,
		}}
		claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{
			PromptTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 50,
			},
		}}

		usage := buildMessageDeltaPatchUsage(claudeResponse, claudeInfo)
		require.EqualValues(t, 9, usage.InputTokens)
		require.EqualValues(t, 7, usage.CacheReadInputTokens)
		require.EqualValues(t, 6, usage.CacheCreationInputTokens)
	})

	t.Run("default aggregate cache creation to 5m when split missing", func(t *testing.T) {
		claudeResponse := &dto.ClaudeResponse{Usage: &dto.ClaudeUsage{
			OutputTokens:             53,
			CacheCreationInputTokens: 50,
		}}
		claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{
			PromptTokensDetails: dto.InputTokenDetails{
				CachedCreationTokens: 50,
			},
		}}

		usage := buildMessageDeltaPatchUsage(claudeResponse, claudeInfo)
		require.NotNil(t, usage)
		require.NotNil(t, usage.CacheCreation)
		require.EqualValues(t, 50, usage.CacheCreation.Ephemeral5mInputTokens)
		require.EqualValues(t, 0, usage.CacheCreation.Ephemeral1hInputTokens)
	})
}

func newGpt56ClaudePatchRelayInfo(model string, passThrough bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: passThrough},
		},
	}
}

func newClaudePatchTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	return ctx, w
}

func extractClaudeStreamData(t *testing.T, body string) string {
	t.Helper()

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("missing data line in stream body: %q", body)
	return ""
}

func TestHandleStreamResponseDataGpt56AnthropicCacheCreationPatch(t *testing.T) {
	originGlobalPassThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originGlobalPassThrough
	})
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	for _, passThrough := range []bool{false, true} {
		t.Run("pass_through="+strconv.FormatBool(passThrough), func(t *testing.T) {
			ctx, recorder := newClaudePatchTestContext(t)
			info := newGpt56ClaudePatchRelayInfo("gpt-5.6-sol", passThrough)
			claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
			data := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":6045,"cache_read_input_tokens":83456,"cache_creation_input_tokens":0,"output_tokens":330}}`

			err := HandleStreamResponseData(ctx, info, claudeInfo, data)

			require.Nil(t, err)
			patchedData := extractClaudeStreamData(t, recorder.Body.String())
			require.EqualValues(t, 6045, gjson.Get(patchedData, "usage.cache_creation_input_tokens").Int())
			require.EqualValues(t, 6045, claudeInfo.Usage.PromptTokens)
			require.EqualValues(t, 83456, claudeInfo.Usage.PromptTokensDetails.CachedTokens)
		})
	}
}

func TestHandleStreamResponseDataGpt56AnthropicCacheCreationPatchGuards(t *testing.T) {
	originGlobalPassThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originGlobalPassThrough
	})
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	t.Run("upstream value wins", func(t *testing.T) {
		ctx, recorder := newClaudePatchTestContext(t)
		info := newGpt56ClaudePatchRelayInfo("gpt-5.6-sol", true)
		claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
		data := `{"type":"message_delta","usage":{"input_tokens":6045,"cache_read_input_tokens":83456,"cache_creation_input_tokens":777,"output_tokens":330}}`

		err := HandleStreamResponseData(ctx, info, claudeInfo, data)

		require.Nil(t, err)
		patchedData := extractClaudeStreamData(t, recorder.Body.String())
		require.EqualValues(t, 777, gjson.Get(patchedData, "usage.cache_creation_input_tokens").Int())
	})

	t.Run("lower model skips", func(t *testing.T) {
		ctx, recorder := newClaudePatchTestContext(t)
		info := newGpt56ClaudePatchRelayInfo("gpt-5.5-sol", true)
		claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
		data := `{"type":"message_delta","usage":{"input_tokens":6045,"cache_read_input_tokens":83456,"cache_creation_input_tokens":0,"output_tokens":330}}`

		err := HandleStreamResponseData(ctx, info, claudeInfo, data)

		require.Nil(t, err)
		patchedData := extractClaudeStreamData(t, recorder.Body.String())
		require.EqualValues(t, 0, gjson.Get(patchedData, "usage.cache_creation_input_tokens").Int())
	})
}

func TestHandleClaudeResponseDataGpt56AnthropicCacheCreationPatch(t *testing.T) {
	ctx, recorder := newClaudePatchTestContext(t)
	info := newGpt56ClaudePatchRelayInfo("gpt-5.6-sol", true)
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}
	data := []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"gpt-5.6-sol","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":6045,"cache_read_input_tokens":83456,"cache_creation_input_tokens":0,"output_tokens":330}}`)

	err := HandleClaudeResponseData(ctx, info, claudeInfo, upstream, data)

	require.Nil(t, err)
	require.EqualValues(t, 6045, gjson.Get(recorder.Body.String(), "usage.cache_creation_input_tokens").Int())
}
