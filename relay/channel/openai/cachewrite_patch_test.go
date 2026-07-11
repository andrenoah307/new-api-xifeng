package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newPatchGpt56CacheWriteInfo(format types.RelayFormat) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     format,
		OriginModelName: "gpt-5.6-terra",
		PriceData: types.PriceData{
			ModelRatio:         1,
			CompletionRatio:    2,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}
}

func newPatchChatUsage() *dto.Usage {
	return &dto.Usage{
		PromptTokens: 1000,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}
}

func newPatchResponsesUsage() *dto.Usage {
	return &dto.Usage{
		InputTokens: 1000,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 300,
			ImageTokens:  40,
			AudioTokens:  10,
		},
	}
}

func TestPatchGpt56CacheWriteStr(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		info       *relaycommon.RelayInfo
		usage      *dto.Usage
		usagePath  string
		detailsKey string
		wantPath   string
		wantValue  int64
		wantSame   bool
	}{
		{
			name:       "chat prompt tokens details",
			data:       `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":300,"image_tokens":40,"audio_tokens":10}}}`,
			info:       newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAI),
			usage:      newPatchChatUsage(),
			usagePath:  "usage",
			detailsKey: "prompt_tokens_details",
			wantPath:   "usage.prompt_tokens_details.cache_write_tokens",
			wantValue:  650,
		},
		{
			name:       "response completed prefix",
			data:       `{"type":"response.completed","response":{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":300,"image_tokens":40,"audio_tokens":10}}}}`,
			info:       newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAIResponses),
			usage:      newPatchResponsesUsage(),
			usagePath:  "response.usage",
			detailsKey: "input_tokens_details",
			wantPath:   "response.usage.input_tokens_details.cache_write_tokens",
			wantValue:  650,
		},
		{
			name:       "upstream cache write wins",
			data:       `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":300,"cache_write_tokens":123}}}`,
			info:       newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAI),
			usage:      newPatchChatUsage(),
			usagePath:  "usage",
			detailsKey: "prompt_tokens_details",
			wantSame:   true,
		},
		{
			name:       "upstream cache creation wins",
			data:       `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":300,"cache_creation_tokens":123}}}`,
			info:       newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAI),
			usage:      newPatchChatUsage(),
			usagePath:  "usage",
			detailsKey: "prompt_tokens_details",
			wantSame:   true,
		},
		{
			name: "guard not passed returns original",
			data: `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":300}}}`,
			info: func() *relaycommon.RelayInfo {
				info := newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAI)
				info.OriginModelName = "gpt-5.5"
				return info
			}(),
			usage:      newPatchChatUsage(),
			usagePath:  "usage",
			detailsKey: "prompt_tokens_details",
			wantSame:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := patchGpt56CacheWriteStr(tt.data, tt.info, tt.usage, tt.usagePath, tt.detailsKey)

			if tt.wantSame {
				require.Equal(t, tt.data, got)
				return
			}
			require.Equal(t, tt.wantValue, gjson.Get(got, tt.wantPath).Int())
		})
	}
}

func TestPatchGpt56CacheWriteBytes(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":300,"image_tokens":40,"audio_tokens":10}}}`)
	got := patchGpt56CacheWriteBytes(data, newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAIResponses), newPatchResponsesUsage(), "usage", "input_tokens_details")

	require.Equal(t, int64(650), gjson.GetBytes(got, "usage.input_tokens_details.cache_write_tokens").Int())
}

func TestPatchGpt56CacheWriteBytesSkips(t *testing.T) {
	guardData := []byte(`{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":300}}}`)
	guardInfo := newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAIResponses)
	guardInfo.PriceData.UsePrice = true
	require.Equal(t, guardData, patchGpt56CacheWriteBytes(guardData, guardInfo, newPatchResponsesUsage(), "usage", "input_tokens_details"))

	upstreamData := []byte(`{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":300,"cache_write_tokens":123}}}`)
	got := patchGpt56CacheWriteBytes(upstreamData, newPatchGpt56CacheWriteInfo(types.RelayFormatOpenAIResponses), newPatchResponsesUsage(), "usage", "input_tokens_details")
	require.Equal(t, upstreamData, got)
}
