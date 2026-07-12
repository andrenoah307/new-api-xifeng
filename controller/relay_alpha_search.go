package controller

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type alphaSearchRequest struct {
	Model string `json:"model"`
}

func computeAlphaSearchQuota(price, groupRatio float64) (int, *common.QuotaClamp) {
	quotaDecimal := decimal.NewFromFloat(price).
		Div(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromFloat(groupRatio)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	return common.QuotaFromDecimalChecked(quotaDecimal)
}

func alphaSearchRequestURL(baseURL string, channelType int) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/alpha/search"
	}
	return relaycommon.GetFullRequestURL(baseURL, "/v1/alpha/search", channelType)
}

func RelayAlphaSearch(c *gin.Context) {
	var request alphaSearchRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}
	if request.Model == "" {
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			fmt.Errorf("model is required"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}

	relayRequest := &dto.GeneralOpenAIRequest{Model: request.Model}
	relayInfo := relaycommon.GenRelayInfoOpenAI(c, relayRequest)
	relayInfo.InitChannelMeta(c)
	relayInfo.ForcePreConsume = true
	groupRatioInfo := helper.HandleGroupRatio(c, relayInfo)
	relayInfo.PriceData.GroupRatioInfo = groupRatioInfo

	price := operation_setting.GetToolPrice("alpha_search")
	if price < 0 || groupRatioInfo.GroupRatio < 0 || math.IsNaN(price) || math.IsNaN(groupRatioInfo.GroupRatio) || math.IsInf(price, 0) || math.IsInf(groupRatioInfo.GroupRatio, 0) {
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			fmt.Errorf("invalid alpha search price or group ratio"),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}

	quota, clamp := computeAlphaSearchQuota(price, groupRatioInfo.GroupRatio)
	relayInfo.PriceData.Quota = quota
	relayInfo.QuotaClamp = clamp
	if quota > 0 {
		if apiErr := service.PreConsumeBilling(c, quota, quota, relayInfo); apiErr != nil {
			writeAlphaSearchError(c, apiErr)
			return
		}
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		refundAlphaSearchBilling(c, relayInfo)
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeReadRequestBodyFailed,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}
	if _, err = storage.Seek(0, io.SeekStart); err != nil {
		refundAlphaSearchBilling(c, relayInfo)
		writeAlphaSearchError(c, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry()))
		return
	}

	upstreamURL := alphaSearchRequestURL(relayInfo.ChannelBaseUrl, relayInfo.ChannelType)
	upstreamRequest, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, storage)
	if err != nil {
		refundAlphaSearchBilling(c, relayInfo)
		writeAlphaSearchError(c, types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry()))
		return
	}
	upstreamRequest.ContentLength = storage.Size()
	upstreamRequest.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	upstreamRequest.Header.Set("Authorization", "Bearer "+relayInfo.ApiKey)

	response, err := service.GetHttpClient().Do(upstreamRequest)
	if err != nil {
		refundAlphaSearchBilling(c, relayInfo)
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeDoRequestFailed,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}
	defer service.CloseResponseBodyGracefully(response)

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		refundAlphaSearchBilling(c, relayInfo)
		writeAlphaSearchError(c, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		))
		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		refundAlphaSearchBilling(c, relayInfo)
		service.IOCopyBytesGracefully(c, response, responseBody)
		return
	}

	if relayInfo.Billing != nil {
		if err = service.SettleBilling(c, relayInfo, quota); err != nil {
			refundAlphaSearchBilling(c, relayInfo)
			writeAlphaSearchError(c, types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry()))
			return
		}
	}

	model.RecordConsumeLog(c, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     0,
		CompletionTokens: 0,
		ModelName:        request.Model,
		TokenName:        c.GetString("token_name"),
		Quota:            quota,
		Content:          "Alpha search, per-call billing",
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(time.Since(relayInfo.StartTime).Seconds()),
		IsStream:         false,
		Group:            relayInfo.UsingGroup,
		Other:            map[string]interface{}{"alpha_search": 1},
	})
	model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
	model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	service.IOCopyBytesGracefully(c, response, responseBody)
}

func refundAlphaSearchBilling(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo.Billing != nil {
		relayInfo.Billing.Refund(c)
	}
}

func writeAlphaSearchError(c *gin.Context, apiErr *types.NewAPIError) {
	logger.LogError(c, apiErr.Error())
	body, err := common.Marshal(gin.H{"error": apiErr.ToOpenAIError()})
	if err != nil {
		c.Data(http.StatusInternalServerError, gin.MIMEJSON, []byte(`{"error":{"message":"failed to encode error response"}}`))
		return
	}
	c.Data(apiErr.StatusCode, gin.MIMEJSON, body)
}
