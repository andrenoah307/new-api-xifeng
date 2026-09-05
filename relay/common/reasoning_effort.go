package common

import (
	apicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

// ResolveClaudeThinkingForLog extracts request-side Claude thinking settings.
func ResolveClaudeThinkingForLog(request *dto.ClaudeRequest) (effort string, budget *int, thinkingType string) {
	if request == nil {
		return "", nil, ""
	}
	effort = request.GetEfforts()
	if request.Thinking == nil {
		return effort, nil, ""
	}
	budget = request.Thinking.BudgetTokens
	if request.Thinking.Type == "enabled" && budget != nil {
		return effort, budget, ""
	}
	return effort, budget, request.Thinking.Type
}

type openAIReasoningLog struct {
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

type openAIThinkingLog struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

type enableThinkingLog struct {
	Enabled      *bool  `json:"enabled,omitempty"`
	Type         string `json:"type,omitempty"`
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

// ResolveOpenAIChatThinkingForLog extracts thinking settings from OpenAI-compatible chat fields.
func ResolveOpenAIChatThinkingForLog(request *dto.GeneralOpenAIRequest) (effort string, budget *int, thinkingType string) {
	if request == nil {
		return "", nil, ""
	}
	if request.ReasoningEffort != "" {
		return request.ReasoningEffort, nil, ""
	}
	if len(request.Reasoning) > 0 {
		var reasoning openAIReasoningLog
		if apicommon.Unmarshal(request.Reasoning, &reasoning) == nil {
			if reasoning.Effort != "" {
				return reasoning.Effort, nil, ""
			}
			if reasoning.MaxTokens != nil {
				if reasoning.Enabled != nil && !*reasoning.Enabled {
					return "", reasoning.MaxTokens, "disabled"
				}
				return "", reasoning.MaxTokens, ""
			}
			if reasoning.Enabled != nil {
				if *reasoning.Enabled {
					return "", nil, "enabled"
				}
				return "", nil, "disabled"
			}
		}
	}
	if len(request.THINKING) > 0 {
		var thinking openAIThinkingLog
		if apicommon.Unmarshal(request.THINKING, &thinking) == nil {
			budget = thinking.BudgetTokens
			if thinking.Type == "enabled" && budget != nil {
				return "", budget, ""
			}
			return "", budget, thinking.Type
		}
	}
	if len(request.EnableThinking) > 0 {
		if apicommon.GetJsonType(request.EnableThinking) == "boolean" {
			var enabled bool
			if apicommon.Unmarshal(request.EnableThinking, &enabled) != nil {
				return "", nil, ""
			}
			if enabled {
				return "", nil, "enabled"
			}
			return "", nil, "disabled"
		}
		if apicommon.GetJsonType(request.EnableThinking) == "object" {
			var enabled enableThinkingLog
			if apicommon.Unmarshal(request.EnableThinking, &enabled) == nil {
				if enabled.Enabled != nil {
					if !*enabled.Enabled {
						return "", nil, "disabled"
					}
					if enabled.Type == "" && enabled.BudgetTokens == nil {
						return "", nil, "enabled"
					}
				}
				if enabled.Type == "enabled" && enabled.BudgetTokens != nil {
					return "", enabled.BudgetTokens, ""
				}
				return "", enabled.BudgetTokens, enabled.Type
			}
		}
	}
	return "", nil, ""
}

// ResolveGeminiThinkingForLog extracts Gemini's request-side thinking config.
func ResolveGeminiThinkingForLog(genConfig *dto.GeminiChatGenerationConfig) (budget *int, thinkingType string) {
	if genConfig == nil || genConfig.ThinkingConfig == nil {
		return nil, ""
	}
	return genConfig.ThinkingConfig.ThinkingBudget, genConfig.ThinkingConfig.ThinkingLevel
}

// ResolveOpenAIReasoningEffortForPassthrough 在「透传请求体」跳过 ConvertOpenAIRequest 时，
// 用与 ConvertOpenAIRequest 相同的优先级（模型后缀 > body 的 reasoning_effort）解析思考等级，
// 供 handler 补写 info.ReasoningEffort，避免使用日志丢失思考等级（坑点 #134）。
func ResolveOpenAIReasoningEffortForPassthrough(upstreamModel, requestEffort string) string {
	if effort, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(upstreamModel); effort != "" {
		return effort
	}
	return requestEffort
}

// ResolveResponsesReasoningEffortForPassthrough 在「透传请求体」跳过 ConvertOpenAIResponsesRequest 时，
// 用与该 convert 相同的优先级（模型后缀 > request.reasoning.effort）解析思考等级，nil 安全（坑点 #134）。
func ResolveResponsesReasoningEffortForPassthrough(model string, reasoning *dto.Reasoning) string {
	effort := ""
	if reasoning != nil {
		effort = reasoning.Effort
	}
	return ResolveOpenAIReasoningEffortForPassthrough(model, effort)
}

// ResolveGeminiReasoningEffortForPassthrough 透传时从模型后缀解析 Gemini 思考等级（坑点 #134）。
func ResolveGeminiReasoningEffortForPassthrough(upstreamModel string) string {
	if _, level, ok := reasoning.TrimEffortSuffix(upstreamModel); ok {
		return level
	}
	return ""
}
