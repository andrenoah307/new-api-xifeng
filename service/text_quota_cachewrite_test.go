package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInputTokenDetailsParsesCacheWriteFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{
			name: "cache_write_tokens",
			body: `{"input_tokens":52006,"input_tokens_details":{"cached_tokens":48128,"cache_write_tokens":3200}}`,
			want: 3200,
		},
		{
			name: "cache_creation_tokens",
			body: `{"input_tokens":52006,"input_tokens_details":{"cached_tokens":48128,"cache_creation_tokens":3200}}`,
			want: 3200,
		},
		{
			name: "larger field wins",
			body: `{"input_tokens":52006,"input_tokens_details":{"cache_write_tokens":100,"cache_creation_tokens":900}}`,
			want: 900,
		},
		{
			name: "missing fields",
			body: `{"input_tokens":52006,"input_tokens_details":{"cached_tokens":48128}}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage dto.Usage
			require.NoError(t, common.UnmarshalJsonStr(tt.body, &usage))
			require.NotNil(t, usage.InputTokensDetails)
			require.Equal(t, tt.want, usage.InputTokensDetails.EffectiveCacheWriteTokens())
		})
	}
}

func TestCacheWriteBillingOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
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
		StartTime: time.Now(),
	}

	newUsage := func(cacheWriteTokens int) *dto.Usage {
		return &dto.Usage{
			PromptTokens:     1000,
			CompletionTokens: 100,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:     300,
				CacheWriteTokens: cacheWriteTokens,
			},
		}
	}

	withCacheWrite := calculateTextQuotaSummary(ctx, relayInfo, newUsage(200))
	withoutCacheWrite := calculateTextQuotaSummary(ctx, relayInfo, newUsage(0))

	require.Equal(t, 200, withCacheWrite.CacheCreationTokens)
	require.Equal(t, 980, withCacheWrite.Quota)
	require.Equal(t, 930, withoutCacheWrite.Quota)
	require.Greater(t, withCacheWrite.Quota, withoutCacheWrite.Quota)
}
