package common

import "github.com/QuantumNous/new-api/setting/reasoning"

// ResolveOpenAIReasoningEffortForPassthrough 在「透传请求体」跳过 ConvertOpenAIRequest 时，
// 用与 ConvertOpenAIRequest 相同的优先级（模型后缀 > body 的 reasoning_effort）解析思考等级，
// 供 handler 补写 info.ReasoningEffort，避免使用日志丢失思考等级（坑点 #134）。
func ResolveOpenAIReasoningEffortForPassthrough(upstreamModel, requestEffort string) string {
	if effort, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(upstreamModel); effort != "" {
		return effort
	}
	return requestEffort
}

// ResolveGeminiReasoningEffortForPassthrough 透传时从模型后缀解析 Gemini 思考等级（坑点 #134）。
func ResolveGeminiReasoningEffortForPassthrough(upstreamModel string) string {
	if _, level, ok := reasoning.TrimEffortSuffix(upstreamModel); ok {
		return level
	}
	return ""
}
