package openai

import (
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// patchGpt56CacheWriteStr 在 gpt-5.6+ 上游漏报缓存写入时，把重构值写回下游响应 JSON 的 cache_write_tokens。
// usagePath: usage 对象前缀（"usage" 或 "response.usage"）；detailsKey: "prompt_tokens_details" 或 "input_tokens_details"。
func patchGpt56CacheWriteStr(data string, info *relaycommon.RelayInfo, usage *dto.Usage, usagePath, detailsKey string) string {
	v, ok, _ := service.ReconstructGpt56CacheWrite(info, usage)
	if !ok {
		return data
	}

	writePath := usagePath + "." + detailsKey + ".cache_write_tokens"
	creationPath := usagePath + "." + detailsKey + ".cache_creation_tokens"
	writeValue := gjson.Get(data, writePath)
	creationValue := gjson.Get(data, creationPath)
	if (writeValue.Exists() && writeValue.Int() > 0) || (creationValue.Exists() && creationValue.Int() > 0) {
		return data
	}

	patchedData, err := sjson.Set(data, writePath, v)
	if err != nil {
		return data
	}
	return patchedData
}

func patchGpt56CacheWriteBytes(data []byte, info *relaycommon.RelayInfo, usage *dto.Usage, usagePath, detailsKey string) []byte {
	v, ok, _ := service.ReconstructGpt56CacheWrite(info, usage)
	if !ok {
		return data
	}

	writePath := usagePath + "." + detailsKey + ".cache_write_tokens"
	creationPath := usagePath + "." + detailsKey + ".cache_creation_tokens"
	writeValue := gjson.GetBytes(data, writePath)
	creationValue := gjson.GetBytes(data, creationPath)
	if (writeValue.Exists() && writeValue.Int() > 0) || (creationValue.Exists() && creationValue.Int() > 0) {
		return data
	}

	patchedData, err := sjson.SetBytes(data, writePath, v)
	if err != nil {
		return data
	}
	return patchedData
}
