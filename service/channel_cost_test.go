package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCostUpstreamContract(t *testing.T) {
	valid := fmt.Sprintf(`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.5,"user_rate_multiplier":0.25,"resolved_rate_multiplier":0.25,"peak_rate_enabled":false,"effective_rate_multiplier":0.25,"observed_at":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	body := valid
	status := http.StatusOK
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/gateway/v1/sub2api/billing", r.URL.Path)
		assert.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
		w.Header().Set("Location", "/redirect")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	previousClient := httpClient
	httpClient = server.Client()
	t.Cleanup(func() { httpClient = previousClient })
	base := server.URL + "/gateway/v1"
	channel := &model.Channel{Key: "test-secret", BaseURL: &base}
	settings := model.ChannelCostSettings{Mode: "auto"}
	ratio, err := FetchChannelCostRatio(context.Background(), channel, settings)
	require.NoError(t, err)
	assert.Equal(t, 0.25, *ratio.ResolvedRate)
	for _, invalid := range []string{strings.Replace(valid, `"resolved_rate_multiplier":0.25`, `"resolved_rate_multiplier":-1`, 1), strings.Replace(valid, `"effective_rate_multiplier":0.25`, `"effective_rate_multiplier":0.5`, 1), strings.Replace(valid, `"billing_scope":"token"`, `"billing_scope":"image"`, 1), `{"usage":{"total":{"cost":1,"actual_cost":0.25}}}`, `not json`} {
		body = invalid
		_, err = FetchChannelCostRatio(context.Background(), channel, settings)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "test-secret")
	}
	body = strings.ReplaceAll(valid, "0.25", "0")
	ratio, err = FetchChannelCostRatio(context.Background(), channel, settings)
	require.Error(t, err, "automatic zero rates must not silently erase cost")
	status = http.StatusFound
	_, err = FetchChannelCostRatio(context.Background(), channel, settings)
	require.Error(t, err, "redirects must not receive credentials")
	channel.ChannelInfo.IsMultiKey = true
	_, err = FetchChannelCostRatio(context.Background(), channel, settings)
	require.Error(t, err, "one account ratio cannot represent a mixed key pool")
}

func TestChannelCostUsesNormalizedCachePriceBeforeGroup(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, group := range []float64{0, 0.5, 2} {
		info := &relaycommon.RelayInfo{
			OriginModelName: "cost-fixture", RelayFormat: types.RelayFormatOpenAI,
			StartTime: time.Now(),
			PriceData: types.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheRatio: 0.1,
				CacheCreationRatio: 1.25, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: group}},
		}
		usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 200,
			PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100, CachedCreationTokens: 50}}
		summary := calculateTextQuotaSummary(c, info, usage)
		// (1000 - 100 - 50) + 100*0.1 + 50*1.25 + 200*2.
		assert.Equal(t, "1322.5", summary.ModelCostQuota.String())
	}
}

func TestChannelCostBeforeCustomerDiscount(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	for _, ratio := range []float64{0, 0.5, 2} {
		info.PriceData.GroupRatioInfo.GroupRatio = ratio
		amount := modelCostUSD(info, decimal.NewFromFloat(common.QuotaPerUnit).Mul(decimal.NewFromFloat(0.123456789)), nil, true)
		require.NotNil(t, amount)
		assert.Equal(t, "0.123456789", *amount)
	}
	assert.Nil(t, modelCostUSD(info, decimal.Zero, nil, false))
	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{QuotaPerUnit: common.QuotaPerUnit}
	assert.Nil(t, modelCostUSD(info, decimal.NewFromInt(10), nil, true), "failed expression settlement is not a known model price")
	amount := modelCostUSD(info, decimal.Zero, &billingexpr.TieredResult{ActualQuotaBeforeGroup: common.QuotaPerUnit}, true)
	require.NotNil(t, amount)
	assert.Equal(t, "1", *amount)
}
