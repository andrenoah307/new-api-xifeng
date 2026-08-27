package openai

import (
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	relaycommon.MarkResponsesOutput(info, &responsesResponse)

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// compute usage
	usagePresent := responsesResponse.Usage != nil && relaycommon.UsageHasAnyTokenData(responsesResponse.Usage)
	usage := relayconvert.UsageFromResponsesUsage(responsesResponse.Usage)
	usage = finalizeResponsesUsage(info, usage, info != nil && info.HasDeliverableOutput, usagePresent)
	if info != nil && info.ZeroChargeGuardTriggered {
		if responsesResponse.Usage == nil {
			responsesResponse.Usage = &dto.Usage{}
		} else {
			*responsesResponse.Usage = dto.Usage{}
		}
		responseBody, err = common.Marshal(&responsesResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	}
	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var usageReported bool
	var responseTextRuneCount int
	var pendingTerminalData string
	var pendingTerminalResponse *dto.ResponsesStreamResponse
	guardEnabled := responsesZeroChargeGuardEnabled(info)

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		relaycommon.MarkResponsesStreamOutput(info, &streamResponse)
		if streamResponse.Type == "response.output_text.delta" {
			responseTextRuneCount += utf8.RuneCountInString(streamResponse.Delta)
		}
		if streamResponse.Response != nil {
			relaycommon.MarkResponsesOutput(info, streamResponse.Response)
			if streamResponse.Response.Usage != nil && relaycommon.UsageHasAnyTokenData(streamResponse.Response.Usage) {
				mapped := relayconvert.UsageFromResponsesUsage(streamResponse.Response.Usage)
				*usage = *mapped
				usageReported = true
			}
		}
		if streamResponse.Type == "error" || streamResponse.Type == "response.failed" {
			patched, originalCode, changed := service.RewriteStreamOverloadErrorCode(data)
			if changed {
				data = patched
				if info != nil {
					if info.StreamOverloadRewrite == nil {
						info.StreamOverloadRewrite = &relaycommon.StreamOverloadRewriteMarker{OriginalCode: originalCode}
					}
					info.StreamOverloadRewrite.Count++
				}
				logger.LogWarn(c, fmt.Sprintf("upstream stream overload error code rewritten: original_code=%s", originalCode))
			}
		}
		isTerminal := streamResponse.Type == "response.completed" || streamResponse.Type == "response.done" || streamResponse.Type == "response.incomplete"
		if isTerminal && guardEnabled {
			// Hold the terminal frame until the usage/output decision is known so
			// a prompt-only or missing usage cannot leak on the wire.
			pendingTerminalData = data
			pendingCopy := streamResponse
			pendingTerminalResponse = &pendingCopy
		} else {
			sendResponsesStreamData(c, streamResponse, data)
		}
		switch streamResponse.Type {
		case "response.completed", "response.done", "response.incomplete":
			if streamResponse.Response != nil && streamResponse.Response.HasImageGenerationCall() {
				c.Set("image_generation_call", true)
				c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
				c.Set("image_generation_call_size", streamResponse.Response.GetSize())
			}
		case dto.ResponsesOutputTypeItemDone:
			// 函数调用处理
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		}
	})

	if !responsesZeroChargeGuardEnabled(info) {
		// Preserve the provider-specific legacy contract for adapters that do
		// not report Responses usage: they historically estimated output text.
		if usage.CompletionTokens == 0 && responseTextRuneCount > 0 {
			usage.CompletionTokens = service.EstimateTokenByRuneCount(responseTextRuneCount)
		}
		if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
			usage.PromptTokens = info.GetEstimatePromptTokens()
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	usage = finalizeResponsesUsage(info, usage, info != nil && info.HasDeliverableOutput, usageReported)
	if pendingTerminalResponse != nil {
		terminalData := pendingTerminalData
		if info != nil && info.ZeroChargeGuardTriggered {
			if pendingTerminalResponse.Response == nil {
				pendingTerminalResponse.Response = &dto.OpenAIResponsesResponse{}
			}
			pendingTerminalResponse.Response.Usage = &dto.Usage{}
			var marshalErr error
			terminalDataBytes, marshalErr := common.Marshal(pendingTerminalResponse)
			if marshalErr == nil {
				terminalData = string(terminalDataBytes)
			}
		}
		sendResponsesStreamData(c, *pendingTerminalResponse, terminalData)
	}

	return usage, nil
}
