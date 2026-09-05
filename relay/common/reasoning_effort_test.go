package common

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestResolveClaudeThinkingForLog(t *testing.T) {
	intPtr := func(value int) *int { return &value }
	tests := []struct {
		name         string
		request      *dto.ClaudeRequest
		effort       string
		budget       *int
		thinkingType string
	}{
		{name: "enabled budget", request: &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: intPtr(32000)}}, budget: intPtr(32000)},
		{name: "enabled without budget", request: &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "enabled"}}, thinkingType: "enabled"},
		{name: "adaptive effort", request: &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "adaptive"}, OutputConfig: json.RawMessage(`{"effort":"high"}`)}, effort: "high", thinkingType: "adaptive"},
		{name: "disabled", request: &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "disabled"}}, thinkingType: "disabled"},
		{name: "explicit zero budget", request: &dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: intPtr(0)}}, budget: intPtr(0)},
		{name: "nil request", request: nil},
		{name: "nil thinking", request: &dto.ClaudeRequest{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effort, budget, thinkingType := ResolveClaudeThinkingForLog(tt.request)
			require.Equal(t, tt.effort, effort)
			require.Equal(t, tt.budget, budget)
			require.Equal(t, tt.thinkingType, thinkingType)
		})
	}
}

func TestResolveOpenAIChatThinkingForLog(t *testing.T) {
	effort, budget, thinkingType := ResolveOpenAIChatThinkingForLog(nil)
	require.Empty(t, effort)
	require.Nil(t, budget)
	require.Empty(t, thinkingType)
	intPtr := func(value int) *int { return &value }

	tests := []struct {
		name         string
		request      *dto.GeneralOpenAIRequest
		effort       string
		budget       *int
		thinkingType string
	}{
		{name: "reasoning effort", request: &dto.GeneralOpenAIRequest{ReasoningEffort: "high"}, effort: "high"},
		{name: "openrouter effort", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{"effort":"low"}`)}, effort: "low"},
		{name: "openrouter max tokens", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{"max_tokens":4096}`)}, budget: intPtr(4096)},
		{name: "openrouter disabled", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{"enabled":false}`)}, thinkingType: "disabled"},
		{name: "openrouter enabled", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{"enabled":true}`)}, thinkingType: "enabled"},
		{name: "openrouter disabled with budget", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{"enabled":false,"max_tokens":1}`)}, budget: intPtr(1), thinkingType: "disabled"},
		{name: "thinking budget", request: &dto.GeneralOpenAIRequest{THINKING: json.RawMessage(`{"type":"enabled","budget_tokens":8192}`)}, budget: intPtr(8192)},
		{name: "thinking enabled without budget", request: &dto.GeneralOpenAIRequest{THINKING: json.RawMessage(`{"type":"enabled"}`)}, thinkingType: "enabled"},
		{name: "enable thinking false", request: &dto.GeneralOpenAIRequest{EnableThinking: json.RawMessage(`false`)}, thinkingType: "disabled"},
		{name: "enable thinking true", request: &dto.GeneralOpenAIRequest{EnableThinking: json.RawMessage(`true`)}, thinkingType: "enabled"},
		{name: "enable thinking object false", request: &dto.GeneralOpenAIRequest{EnableThinking: json.RawMessage(`{"enabled":false}`)}, thinkingType: "disabled"},
		{name: "enable thinking object budget", request: &dto.GeneralOpenAIRequest{EnableThinking: json.RawMessage(`{"enabled":true,"budget_tokens":4}`)}, budget: intPtr(4)},
		{name: "enable thinking malformed object", request: &dto.GeneralOpenAIRequest{EnableThinking: json.RawMessage(`{"enabled":"true"}`)}},
		{name: "malformed fields", request: &dto.GeneralOpenAIRequest{Reasoning: json.RawMessage(`{`), THINKING: json.RawMessage(`[`), EnableThinking: json.RawMessage(`{`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effort, budget, thinkingType := ResolveOpenAIChatThinkingForLog(tt.request)
			require.Equal(t, tt.effort, effort)
			require.Equal(t, tt.budget, budget)
			require.Equal(t, tt.thinkingType, thinkingType)
		})
	}
}

func TestResolveGeminiThinkingForLog(t *testing.T) {
	budget, thinkingType := ResolveGeminiThinkingForLog(nil)
	require.Nil(t, budget)
	require.Empty(t, thinkingType)
	budget, thinkingType = ResolveGeminiThinkingForLog(&dto.GeminiChatGenerationConfig{})
	require.Nil(t, budget)
	require.Empty(t, thinkingType)

	tests := []struct {
		name   string
		budget int
	}{
		{name: "explicit zero", budget: 0},
		{name: "dynamic", budget: -1},
		{name: "positive budget", budget: 8192},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := tt.budget
			gotBudget, gotType := ResolveGeminiThinkingForLog(&dto.GeminiChatGenerationConfig{ThinkingConfig: &dto.GeminiThinkingConfig{ThinkingBudget: &budget}})
			require.Equal(t, &budget, gotBudget)
			require.Empty(t, gotType)
		})
	}
	budget, thinkingType = ResolveGeminiThinkingForLog(&dto.GeminiChatGenerationConfig{ThinkingConfig: &dto.GeminiThinkingConfig{ThinkingLevel: "high"}})
	require.Nil(t, budget)
	require.Equal(t, "high", thinkingType)
}

func TestResolveOpenAIReasoningEffortForPassthrough(t *testing.T) {
	tests := []struct {
		name          string
		upstreamModel string
		requestEffort string
		want          string
	}{
		{
			name:          "model suffix wins when body empty",
			upstreamModel: "gpt-5-high",
			requestEffort: "",
			want:          "high",
		},
		{
			name:          "falls back to body reasoning effort",
			upstreamModel: "gpt-5",
			requestEffort: "medium",
			want:          "medium",
		},
		{
			name:          "model suffix takes precedence over body",
			upstreamModel: "gpt-5-high",
			requestEffort: "low",
			want:          "high",
		},
		{
			name:          "no reasoning effort",
			upstreamModel: "gpt-4o",
			requestEffort: "",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOpenAIReasoningEffortForPassthrough(tt.upstreamModel, tt.requestEffort)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveResponsesReasoningEffortForPassthrough(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		reasoning *dto.Reasoning
		want      string
	}{
		{
			name:      "model suffix wins with nil reasoning",
			model:     "gpt-5-high",
			reasoning: nil,
			want:      "high",
		},
		{
			name:      "falls back to body reasoning effort",
			model:     "gpt-5",
			reasoning: &dto.Reasoning{Effort: "medium"},
			want:      "medium",
		},
		{
			name:      "model suffix takes precedence over body",
			model:     "gpt-5-high",
			reasoning: &dto.Reasoning{Effort: "low"},
			want:      "high",
		},
		{
			name:      "no reasoning effort with nil reasoning",
			model:     "gpt-4o",
			reasoning: nil,
			want:      "",
		},
		{
			name:      "empty body reasoning effort is ignored",
			model:     "gpt-4o",
			reasoning: &dto.Reasoning{Effort: ""},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveResponsesReasoningEffortForPassthrough(tt.model, tt.reasoning)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveGeminiReasoningEffortForPassthrough(t *testing.T) {
	tests := []struct {
		name          string
		upstreamModel string
		want          string
	}{
		{
			name:          "model suffix resolves level",
			upstreamModel: "gemini-2.5-pro-high",
			want:          "high",
		},
		{
			name:          "no suffix returns empty",
			upstreamModel: "gemini-2.5-pro",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveGeminiReasoningEffortForPassthrough(tt.upstreamModel)
			require.Equal(t, tt.want, got)
		})
	}
}
