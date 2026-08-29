package common

import (
	"testing"

	apicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseoutZeroChargeClearsUsageAndCapturesFiniteSnapshot(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     1601,
		CompletionTokens: 0,
		TotalTokens:      1601,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         17,
			CachedCreationTokens: 23,
			CacheWriteTokens:     29,
			TextTokens:           31,
			AudioTokens:          37,
			ImageTokens:          41,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{
			ReasoningTokens: 43,
			TextTokens:      47,
			AudioTokens:     53,
			ImageTokens:     59,
		},
		InputTokens:                 1601,
		OutputTokens:                0,
		InputTokensDetails:          &dto.InputTokenDetails{CachedTokens: 61},
		ClaudeCacheCreation5mTokens: 67,
		ClaudeCacheCreation1hTokens: 71,
		PromptCacheHitTokens:        73,
		Cost:                        "untrusted-cost",
		BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
			InputTokens:              1601,
			OutputTokens:             553250,
			CacheReadInputTokens:     17,
			CacheCreationInputTokens: 23,
		}),
	}
	info := &RelayInfo{FinalPreConsumedQuota: 99}

	got := CloseoutZeroCharge(info, usage, ZeroChargeReasonEmptyOutput)

	require.Same(t, usage, got)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.Nil(t, usage.BillingUsage)
	require.True(t, info.ZeroChargeGuardTriggered)
	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, ZeroChargeReasonEmptyOutput, info.ZeroChargeGuardSnapshot.Reason)
	assert.Equal(t, 1601, info.ZeroChargeGuardSnapshot.PromptTokens)
	assert.Equal(t, 553250, info.ZeroChargeGuardSnapshot.CompletionTokens)
	assert.Equal(t, 73, info.ZeroChargeGuardSnapshot.CacheReadTokens)
	assert.Equal(t, 138, info.ZeroChargeGuardSnapshot.CacheCreationTokens)
	assert.Equal(t, 99, info.ZeroChargeGuardSnapshot.PreConsumedQuota)
}

func TestCloseoutZeroChargeBoundsSnapshotCounters(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	usage := &dto.Usage{
		PromptTokens:                apicommon.MaxQuota + 1,
		ClaudeCacheCreation5mTokens: maxInt,
		ClaudeCacheCreation1hTokens: 1,
		BillingUsage: &dto.BillingUsage{
			ClaudeUsage: &dto.ClaudeUsage{
				InputTokens:              maxInt,
				OutputTokens:             maxInt,
				CacheCreationInputTokens: maxInt,
			},
		},
	}
	info := &RelayInfo{}

	CloseoutZeroCharge(info, usage, ZeroChargeReasonEmptyOutput)

	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, apicommon.MaxQuota, info.ZeroChargeGuardSnapshot.PromptTokens)
	assert.Equal(t, apicommon.MaxQuota, info.ZeroChargeGuardSnapshot.CompletionTokens)
	assert.Equal(t, apicommon.MaxQuota, info.ZeroChargeGuardSnapshot.CacheCreationTokens)
}

func TestCloseoutZeroChargeSnapshotsNestedResponsesCacheDetails(t *testing.T) {
	nested := &dto.Usage{
		InputTokens: 100,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens:         17,
			CachedCreationTokens: 23,
			CacheWriteTokens:     29,
		},
	}
	usage := &dto.Usage{BillingUsage: &dto.BillingUsage{
		Source:      dto.BillingUsageSourceOAIResponses,
		Semantic:    dto.BillingUsageSemanticOpenAI,
		OpenAIUsage: nested,
	}}
	info := &RelayInfo{}
	nestedBefore := *nested
	nestedDetailsBefore := *nested.InputTokensDetails

	CloseoutZeroCharge(info, usage, ZeroChargeReasonUsageMissing)

	require.NotNil(t, info.ZeroChargeGuardSnapshot)
	assert.Equal(t, 17, info.ZeroChargeGuardSnapshot.CacheReadTokens)
	assert.Equal(t, 29, info.ZeroChargeGuardSnapshot.CacheCreationTokens)
	assert.Equal(t, dto.Usage{}, *usage)
	assert.Equal(t, nestedBefore, *nested)
	assert.Equal(t, nestedDetailsBefore, *nested.InputTokensDetails)
}

func TestCloseoutZeroChargeAllocatesUsageAndUsesFiniteReason(t *testing.T) {
	info := &RelayInfo{}
	got := CloseoutZeroCharge(info, nil, ZeroChargeReasonUsageMissing)

	require.NotNil(t, got)
	assert.Equal(t, dto.Usage{}, *got)
	assert.True(t, info.ZeroChargeGuardTriggered)
	assert.Equal(t, ZeroChargeReasonUsageMissing, info.ZeroChargeGuardSnapshot.Reason)
}

func TestResetAttemptUsageStateDoesNotLeakMarkerOrOutput(t *testing.T) {
	info := &RelayInfo{}
	CloseoutZeroCharge(info, &dto.Usage{PromptTokens: 100}, ZeroChargeReasonEmptyOutput)
	info.MarkDeliverableOutput(42)
	require.True(t, info.ZeroChargeGuardTriggered)
	require.True(t, info.HasDeliverableOutput)

	info.ResetAttemptUsageState()

	assert.False(t, info.ZeroChargeGuardTriggered)
	assert.Nil(t, info.ZeroChargeGuardSnapshot)
	assert.False(t, info.HasDeliverableOutput)
	assert.Zero(t, info.OutputRuneCount)

	// The next attempt can now establish an independent, billable output state.
	info.MarkDeliverableOutput(3)
	assert.True(t, info.HasDeliverableOutput)
	assert.Equal(t, 3, info.OutputRuneCount)
	assert.False(t, info.ZeroChargeGuardTriggered)
	assert.Nil(t, info.ZeroChargeGuardSnapshot)
}

func TestChatCompletionsOutputContract(t *testing.T) {
	empty := ""
	reasoning := "think"
	tests := []struct {
		name string
		resp dto.ChatCompletionsStreamResponse
		want bool
	}{
		{
			name: "role only",
			resp: dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}}}},
		},
		{
			name: "finish only",
			resp: dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &empty}}}},
		},
		{
			name: "text",
			resp: dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: strPtr("hello")}}}},
			want: true,
		},
		{
			name: "reasoning",
			resp: dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ReasoningContent: &reasoning}}}},
			want: true,
		},
		{
			name: "empty tool call is output",
			resp: dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{}}}}}},
			want: true,
		},
		{
			name: "usage only",
			resp: dto.ChatCompletionsStreamResponse{Usage: &dto.Usage{PromptTokens: 10}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasChatCompletionsOutput(&tt.resp))
		})
	}
}

func TestOpenAIResponseOutputContractAndDetails(t *testing.T) {
	empty := ""
	tests := []struct {
		name  string
		resp  *dto.OpenAITextResponse
		usage *dto.Usage
		want  bool
	}{
		{name: "empty response", resp: &dto.OpenAITextResponse{}, want: false},
		{name: "text", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{Content: "ok"}}}}, want: true},
		{name: "reasoning", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{ReasoningContent: strPtr("think")}}}}, want: true},
		{name: "empty tool json is not output", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{ToolCalls: []byte(`[]`)}}}}, want: false},
		{name: "null tool json is not output", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{ToolCalls: []byte(`null`)}}}}, want: false},
		{name: "tool call exists", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{ToolCalls: []byte(`[{"id":"call_1"}]`)}}}}, want: true},
		{name: "media output", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{Content: []any{map[string]any{"type": dto.ContentTypeImageURL, "image_url": map[string]any{"url": "data:image/png;base64,AA"}}}}}}}, want: true},
		{name: "output details", resp: &dto.OpenAITextResponse{Choices: []dto.OpenAITextResponseChoice{{Message: dto.Message{Content: &empty}}}}, usage: &dto.Usage{CompletionTokenDetails: dto.OutputTokenDetails{AudioTokens: 1}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasOpenAIResponseOutput(tt.resp, tt.usage))
		})
	}
}

func TestResponsesOutputContract(t *testing.T) {
	tests := []struct {
		name string
		resp *dto.OpenAIResponsesResponse
		want bool
	}{
		{name: "created or empty", resp: &dto.OpenAIResponsesResponse{}, want: false},
		{name: "message text", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "message", Role: "assistant", Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: "hello"}}}}}, want: true},
		{name: "reasoning summary", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "reasoning", Content: []dto.ResponsesOutputContent{{Type: "summary_text", Text: "think"}}}}}, want: true},
		{name: "reasoning summary field", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "reasoning", Summary: []dto.ResponsesReasoningSummaryPart{{Type: "summary_text", Text: "think"}}}}}, want: true},
		{name: "function tool with empty fields", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "function_call"}}}, want: true},
		{name: "custom tool", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "custom_tool_call"}}}, want: true},
		{name: "image", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: dto.ResponsesOutputTypeImageGenerationCall}}}, want: true},
		{name: "empty message", resp: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "message", Role: "assistant"}}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasResponsesOutput(tt.resp))
		})
	}
}

func TestResponsesStreamOutputContract(t *testing.T) {
	tests := []struct {
		name  string
		event dto.ResponsesStreamResponse
		want  bool
	}{
		{name: "created", event: dto.ResponsesStreamResponse{Type: "response.created"}},
		{name: "text delta", event: dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: "x"}, want: true},
		{name: "text done", event: dto.ResponsesStreamResponse{Type: "response.output_text.done", Text: "final"}, want: true},
		{name: "reasoning delta", event: dto.ResponsesStreamResponse{Type: "response.reasoning_summary_text.delta", Delta: "x"}, want: true},
		{name: "reasoning done", event: dto.ResponsesStreamResponse{Type: "response.reasoning_summary_text.done", Text: "final"}, want: true},
		{name: "function item", event: dto.ResponsesStreamResponse{Type: "response.output_item.added", Item: &dto.ResponsesOutput{Type: "function_call"}}, want: true},
		{name: "custom args done", event: dto.ResponsesStreamResponse{Type: "response.custom_tool_call_input.done"}, want: true},
		{name: "usage only completed", event: dto.ResponsesStreamResponse{Type: "response.completed", Response: &dto.OpenAIResponsesResponse{Usage: &dto.Usage{InputTokens: 2}}}},
		{name: "terminal tool output", event: dto.ResponsesStreamResponse{Type: "response.done", Response: &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{Type: "function_call"}}}}, want: true},
		{name: "built-in search done", event: dto.ResponsesStreamResponse{Type: dto.ResponsesOutputTypeItemDone, Item: &dto.ResponsesOutput{Type: dto.BuildInCallWebSearchCall}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasResponsesStreamOutput(&tt.event))
		})
	}
}

func TestMarkOutputOnlyMarksAfterSuccessfulParse(t *testing.T) {
	info := &RelayInfo{}
	chunk := &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{}}}}}}
	MarkChatCompletionsOutput(info, chunk)
	assert.True(t, info.HasDeliverableOutput)
	assert.Equal(t, 0, info.OutputRuneCount)
}

func TestCompletionsOutputContract(t *testing.T) {
	var response dto.CompletionsStreamResponse
	require.NoError(t, apicommon.UnmarshalJsonStr(`{"choices":[{"text":""},{"text":"ok"}]}`, &response))
	assert.True(t, HasCompletionsStreamOutput(&response))
	var empty dto.CompletionsStreamResponse
	require.NoError(t, apicommon.UnmarshalJsonStr(`{"choices":[{"text":""}]}`, &empty))
	assert.False(t, HasCompletionsStreamOutput(&empty))
}

func TestUsageHasAnyTokenDataRecognizesCacheOnlyUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage *dto.Usage
		want  bool
	}{
		{name: "nil", usage: nil, want: false},
		{name: "empty", usage: &dto.Usage{}, want: false},
		{
			name:  "cached input only",
			usage: &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 17}},
			want:  true,
		},
		{
			name:  "cache creation only",
			usage: &dto.Usage{ClaudeCacheCreation5mTokens: 23},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UsageHasAnyTokenData(tt.usage))
		})
	}
}

func TestGeminiOutputContractIncludesThoughtParts(t *testing.T) {
	response := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{
		Parts: []dto.GeminiPart{{Thought: true}},
	}}}}
	assert.True(t, HasGeminiResponseOutput(response))
}

func TestClaudeOutputContractIncludesServerToolsAndNestedBlocks(t *testing.T) {
	serverTool := &dto.ClaudeServerToolUse{WebSearchRequests: 1}
	assert.True(t, HasClaudeResponseOutput(&dto.ClaudeResponse{
		Usage: &dto.ClaudeUsage{ServerToolUse: serverTool},
	}))
	assert.True(t, HasClaudeResponseOutput(&dto.ClaudeResponse{
		Message: &dto.ClaudeMediaMessage{
			Content: []any{map[string]any{"type": "tool_use"}},
		},
	}))
}

func strPtr(value string) *string { return &value }
