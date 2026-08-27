package common

import (
	"math"
	"strings"
	"unicode/utf8"

	apicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// ZeroChargeReason is deliberately a small, stable enum because it is written
// into administrator-only consume-log metadata.
type ZeroChargeReason string

const (
	ZeroChargeReasonEmptyOutput  ZeroChargeReason = "empty_output"
	ZeroChargeReasonUsageMissing ZeroChargeReason = "usage_missing"
)

// ZeroChargeGuardSnapshot contains only bounded token counters. It never keeps
// a response body or a complete BillingUsage object.
type ZeroChargeGuardSnapshot struct {
	Reason              ZeroChargeReason `json:"reason"`
	PromptTokens        int              `json:"prompt_tokens"`
	CompletionTokens    int              `json:"completion_tokens"`
	CacheReadTokens     int              `json:"cache_read_tokens"`
	CacheCreationTokens int              `json:"cache_creation_tokens"`
	PreConsumedQuota    int              `json:"pre_consumed_quota,omitempty"`
}

func (s ZeroChargeGuardSnapshot) AuditMap() map[string]interface{} {
	marker := map[string]interface{}{
		"reason":                string(s.Reason),
		"prompt_tokens":         s.PromptTokens,
		"completion_tokens":     s.CompletionTokens,
		"cache_read_tokens":     s.CacheReadTokens,
		"cache_creation_tokens": s.CacheCreationTokens,
	}
	if s.PreConsumedQuota > 0 {
		marker["pre_consumed_quota"] = s.PreConsumedQuota
	}
	return marker
}

func validZeroChargeReason(reason ZeroChargeReason) bool {
	return reason == ZeroChargeReasonEmptyOutput || reason == ZeroChargeReasonUsageMissing
}

// CloseoutZeroCharge is the single billing closeout for an empty/untrusted
// output. It snapshots the small set of raw counters before replacing the
// entire usage value, including nested BillingUsage and Cost.
func CloseoutZeroCharge(info *RelayInfo, usage *dto.Usage, reason ZeroChargeReason) *dto.Usage {
	if usage == nil {
		usage = &dto.Usage{}
	}
	if !validZeroChargeReason(reason) {
		reason = ZeroChargeReasonUsageMissing
	}
	snapshot := snapshotUsage(usage)
	snapshot.Reason = reason
	if info != nil && info.FinalPreConsumedQuota > 0 {
		snapshot.PreConsumedQuota = boundedNonNegative(info.FinalPreConsumedQuota)
	}
	if info != nil {
		info.ZeroChargeGuardTriggered = true
		info.ZeroChargeGuardSnapshot = &snapshot
	}
	*usage = dto.Usage{}
	return usage
}

func snapshotUsage(usage *dto.Usage) ZeroChargeGuardSnapshot {
	if usage == nil {
		return ZeroChargeGuardSnapshot{}
	}
	snapshot := ZeroChargeGuardSnapshot{
		PromptTokens:        boundedNonNegative(maxInt(usage.PromptTokens, usage.InputTokens)),
		CompletionTokens:    boundedNonNegative(maxInt(usage.CompletionTokens, usage.OutputTokens)),
		CacheReadTokens:     boundedNonNegative(maxInt(usage.PromptTokensDetails.CachedTokens, usage.PromptCacheHitTokens)),
		CacheCreationTokens: boundedNonNegative(maxInt(usage.PromptTokensDetails.CacheCreationTokensTotal(), saturatingAdd(usage.ClaudeCacheCreation5mTokens, usage.ClaudeCacheCreation1hTokens))),
	}

	if usage.BillingUsage == nil {
		return snapshot
	}
	if nested := usage.BillingUsage.OpenAIUsage; nested != nil {
		nestedSnapshot := snapshotUsage(nested)
		snapshot.PromptTokens = boundedNonNegative(maxInt(snapshot.PromptTokens, nestedSnapshot.PromptTokens))
		snapshot.CompletionTokens = boundedNonNegative(maxInt(snapshot.CompletionTokens, nestedSnapshot.CompletionTokens))
		snapshot.CacheReadTokens = boundedNonNegative(maxInt(snapshot.CacheReadTokens, nestedSnapshot.CacheReadTokens))
		snapshot.CacheCreationTokens = boundedNonNegative(maxInt(snapshot.CacheCreationTokens, nestedSnapshot.CacheCreationTokens))
	}
	if nested := usage.BillingUsage.ClaudeUsage; nested != nil {
		cacheCreation := maxInt(nested.CacheCreationInputTokens,
			saturatingAdd(nested.ClaudeCacheCreation5mTokens, nested.ClaudeCacheCreation1hTokens))
		if nested.CacheCreation != nil {
			cacheCreation = maxInt(cacheCreation,
				saturatingAdd(nested.CacheCreation.Ephemeral5mInputTokens, nested.CacheCreation.Ephemeral1hInputTokens))
		}
		snapshot.PromptTokens = boundedNonNegative(maxInt(snapshot.PromptTokens, nested.InputTokens))
		snapshot.CompletionTokens = boundedNonNegative(maxInt(snapshot.CompletionTokens, nested.OutputTokens))
		snapshot.CacheReadTokens = boundedNonNegative(maxInt(snapshot.CacheReadTokens, nested.CacheReadInputTokens))
		snapshot.CacheCreationTokens = boundedNonNegative(maxInt(snapshot.CacheCreationTokens, cacheCreation))
	}
	if nested := usage.BillingUsage.GeminiUsageMetadata; nested != nil {
		prompt := saturatingAdd(nested.PromptTokenCount, nested.ToolUsePromptTokenCount)
		completion := saturatingAdd(nested.CandidatesTokenCount, nested.ThoughtsTokenCount)
		snapshot.PromptTokens = boundedNonNegative(maxInt(snapshot.PromptTokens, prompt))
		snapshot.CompletionTokens = boundedNonNegative(maxInt(snapshot.CompletionTokens, completion))
		snapshot.CacheReadTokens = boundedNonNegative(maxInt(snapshot.CacheReadTokens, nested.CachedContentTokenCount))
	}
	return snapshot
}

func boundedNonNegative(value int) int {
	if value <= 0 {
		return 0
	}
	if value > apicommon.MaxQuota {
		return apicommon.MaxQuota
	}
	return value
}

func maxInt(values ...int) int {
	maximum := 0
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func saturatingAdd(left, right int) int {
	if left <= 0 {
		return boundedNonNegative(right)
	}
	if right <= 0 || left > math.MaxInt-right {
		return boundedNonNegative(left)
	}
	return boundedNonNegative(left + right)
}

// ResetAttemptUsageState must be called at the beginning of every upstream
// attempt because controller/relay.go reuses RelayInfo across retries.
func (info *RelayInfo) ResetAttemptUsageState() {
	if info == nil {
		return
	}
	info.ZeroChargeGuardTriggered = false
	info.ZeroChargeGuardSnapshot = nil
	info.HasDeliverableOutput = false
	info.OutputRuneCount = 0
}

func (info *RelayInfo) MarkDeliverableOutput(runeCount int) {
	if info == nil {
		return
	}
	info.HasDeliverableOutput = true
	if runeCount <= 0 {
		return
	}
	if runeCount >= apicommon.MaxQuota || info.OutputRuneCount > apicommon.MaxQuota-runeCount {
		info.OutputRuneCount = apicommon.MaxQuota
		return
	}
	info.OutputRuneCount += runeCount
}

func HasChatCompletionsOutput(response *dto.ChatCompletionsStreamResponse) bool {
	if response == nil {
		return false
	}
	for index := range response.Choices {
		delta := &response.Choices[index].Delta
		if delta.GetContentString() != "" || delta.GetReasoningContent() != "" || len(delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func HasCompletionsStreamOutput(response *dto.CompletionsStreamResponse) bool {
	if response == nil {
		return false
	}
	for _, choice := range response.Choices {
		if choice.Text != "" {
			return true
		}
	}
	return false
}

func HasOpenAIResponseOutput(response *dto.OpenAITextResponse, usage *dto.Usage) bool {
	if usageHasOutputTokens(usage) {
		return true
	}
	if response == nil {
		return false
	}
	for index := range response.Choices {
		message := &response.Choices[index].Message
		if message.GetReasoningContent() != "" || message.StringContent() != "" || messageHasMedia(message) || validToolCalls(message.ToolCalls) {
			return true
		}
	}
	return false
}

func MarkOpenAIResponseOutput(info *RelayInfo, response *dto.OpenAITextResponse) {
	if info == nil || !HasOpenAIResponseOutput(response, nil) {
		return
	}
	runes := 0
	if response != nil {
		for _, choice := range response.Choices {
			runes = saturatingAdd(runes, utf8.RuneCountInString(choice.Message.StringContent()))
			runes = saturatingAdd(runes, utf8.RuneCountInString(choice.Message.GetReasoningContent()))
		}
	}
	info.MarkDeliverableOutput(runes)
}

func usageHasOutputTokens(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.CompletionTokens > 0 || usage.OutputTokens > 0 ||
		usage.CompletionTokenDetails.TextTokens > 0 ||
		usage.CompletionTokenDetails.AudioTokens > 0 ||
		usage.CompletionTokenDetails.ImageTokens > 0 ||
		usage.CompletionTokenDetails.ReasoningTokens > 0
}

// UsageHasOutputTokens reports output-side counters reported on the primary
// usage object. Nested BillingUsage is deliberately excluded: a stale nested
// value is exactly what the zero-charge closeout must be able to invalidate.
func UsageHasOutputTokens(usage *dto.Usage) bool {
	return usageHasOutputTokens(usage)
}

// UsageHasAnyTokenData distinguishes a genuinely absent/empty usage object
// from a usage value carrying only input or cache counters.
func UsageHasAnyTokenData(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.TotalTokens != 0 || usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.PromptCacheHitTokens != 0 ||
		usage.PromptTokensDetails != (dto.InputTokenDetails{}) || usage.CompletionTokenDetails != (dto.OutputTokenDetails{}) || usage.InputTokensDetails != nil || usage.ClaudeCacheCreation5mTokens != 0 || usage.ClaudeCacheCreation1hTokens != 0 || usage.Cost != nil {
		return true
	}
	if usage.BillingUsage == nil {
		return false
	}
	if usage.BillingUsage.OpenAIUsage != nil || usage.BillingUsage.ClaudeUsage != nil || usage.BillingUsage.GeminiUsageMetadata != nil {
		return true
	}
	return false
}

func validToolCalls(raw []byte) bool {
	if len(strings.TrimSpace(string(raw))) == 0 || strings.TrimSpace(string(raw)) == "null" || strings.TrimSpace(string(raw)) == "[]" {
		return false
	}
	var calls []dto.ToolCallResponse
	if err := apicommon.Unmarshal(raw, &calls); err != nil {
		return false
	}
	return len(calls) > 0
}

func messageHasMedia(message *dto.Message) bool {
	if message == nil || message.Content == nil {
		return false
	}
	if content, ok := message.Content.([]dto.MediaContent); ok {
		for _, item := range content {
			if item.Type != dto.ContentTypeText {
				return true
			}
		}
		return false
	}
	content := message.ParseContent()
	for _, item := range content {
		if item.Type != dto.ContentTypeText {
			return true
		}
	}
	return false
}

func HasResponsesOutput(response *dto.OpenAIResponsesResponse) bool {
	if response == nil {
		return false
	}
	for _, output := range response.Output {
		switch output.Type {
		case "function_call", "custom_tool_call", "file_search_call", "computer_call", "computer_screenshot", "code_interpreter_call", "local_shell_call", "shell_call", dto.ResponsesOutputTypeImageGenerationCall, dto.BuildInCallWebSearchCall:
			return true
		case "message":
			for _, content := range output.Content {
				if content.Text != "" || (content.Type != "" && content.Type != "output_text") {
					return true
				}
			}
		case "reasoning":
			for _, content := range output.Content {
				if content.Text != "" || content.Type != "" {
					return true
				}
			}
			for _, summary := range output.Summary {
				if summary.Text != "" || summary.Type != "" {
					return true
				}
			}
		}
	}
	return false
}

func HasResponsesStreamOutput(event *dto.ResponsesStreamResponse) bool {
	if event == nil {
		return false
	}
	switch event.Type {
	case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		return event.Delta != ""
	case "response.output_text.done":
		return event.Delta != "" || event.Text != ""
	case "response.reasoning_summary_text.done", "response.reasoning_text.done":
		return event.Delta != "" || event.Text != "" || event.Part != nil
	case "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done":
		return true
	case dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone:
		if event.Item == nil {
			return false
		}
		switch event.Item.Type {
		case "function_call", "custom_tool_call", "file_search_call", "computer_call", "computer_screenshot", "code_interpreter_call", "local_shell_call", "shell_call", dto.BuildInCallWebSearchCall, dto.ResponsesOutputTypeImageGenerationCall:
			return true
		case "message":
			return HasResponsesOutput(&dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{*event.Item}})
		case "reasoning":
			return HasResponsesOutput(&dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{*event.Item}})
		}
	case "response.done", "response.completed", "response.incomplete":
		return HasResponsesOutput(event.Response)
	}
	return false
}

func MarkResponsesOutput(info *RelayInfo, response *dto.OpenAIResponsesResponse) {
	if info == nil || !HasResponsesOutput(response) {
		return
	}
	runes := 0
	if response != nil {
		for _, output := range response.Output {
			for _, content := range output.Content {
				runes = saturatingAdd(runes, utf8.RuneCountInString(content.Text))
			}
			for _, summary := range output.Summary {
				runes = saturatingAdd(runes, utf8.RuneCountInString(summary.Text))
			}
		}
	}
	info.MarkDeliverableOutput(runes)
}

func HasGeminiResponseOutput(response *dto.GeminiChatResponse) bool {
	if response == nil {
		return false
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" || part.Thought || rawJSONHasValue(part.ThoughtSignature) || part.FunctionCall != nil || part.FunctionResponse != nil || part.InlineData != nil || part.FileData != nil || part.ExecutableCode != nil || part.CodeExecutionResult != nil {
				return true
			}
		}
	}
	return false
}

func rawJSONHasValue(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "[]" && trimmed != "{}" && trimmed != `""`
}

func HasClaudeResponseOutput(response *dto.ClaudeResponse) bool {
	if response == nil {
		return false
	}
	if response.Completion != "" {
		return true
	}
	if response.Usage != nil && response.Usage.ServerToolUse != nil && response.Usage.ServerToolUse.WebSearchRequests > 0 {
		return true
	}
	if claudeMediaHasOutput(response.Content) || claudeMediaPointerHasOutput(response.ContentBlock) || claudeMediaPointerHasOutput(response.Delta) || claudeMediaPointerHasOutput(response.Message) {
		return true
	}
	return false
}

func claudeMediaPointerHasOutput(message *dto.ClaudeMediaMessage) bool {
	if message == nil {
		return false
	}
	return claudeMediaHasOutput([]dto.ClaudeMediaMessage{*message})
}

func claudeMediaHasOutput(messages []dto.ClaudeMediaMessage) bool {
	for _, message := range messages {
		if claudeMediaTypeIsOutput(message.Type) {
			return true
		}
		if message.Delta != "" || message.GetText() != "" || message.GetStringContent() != "" || message.Thinking != nil && *message.Thinking != "" || message.PartialJson != nil {
			return true
		}
		if nested := message.ParseMediaContent(); len(nested) > 0 && claudeMediaHasOutput(nested) {
			return true
		}
	}
	return false
}

func claudeMediaTypeIsOutput(messageType string) bool {
	switch messageType {
	case "tool_use", "server_tool_use", "input_json_delta", "tool_result":
		return true
	default:
		return false
	}
}

func MarkChatCompletionsOutput(info *RelayInfo, response *dto.ChatCompletionsStreamResponse) {
	if info == nil || !HasChatCompletionsOutput(response) {
		return
	}
	runes := 0
	for _, choice := range response.Choices {
		runes = saturatingAdd(runes, utf8.RuneCountInString(choice.Delta.GetContentString()))
		runes = saturatingAdd(runes, utf8.RuneCountInString(choice.Delta.GetReasoningContent()))
	}
	info.MarkDeliverableOutput(runes)
}

func MarkResponsesStreamOutput(info *RelayInfo, event *dto.ResponsesStreamResponse) {
	if info == nil || !HasResponsesStreamOutput(event) {
		return
	}
	runes := 0
	if event != nil {
		runes = saturatingAdd(runes, utf8.RuneCountInString(event.Delta))
		runes = saturatingAdd(runes, utf8.RuneCountInString(event.Text))
		runes = saturatingAdd(runes, utf8.RuneCountInString(event.Arguments))
		runes = saturatingAdd(runes, utf8.RuneCountInString(event.Input))
		if event.Response != nil {
			for _, output := range event.Response.Output {
				for _, content := range output.Content {
					runes = saturatingAdd(runes, utf8.RuneCountInString(content.Text))
				}
				for _, summary := range output.Summary {
					runes = saturatingAdd(runes, utf8.RuneCountInString(summary.Text))
				}
			}
		}
	}
	info.MarkDeliverableOutput(runes)
}

func MarkGeminiResponseOutput(info *RelayInfo, response *dto.GeminiChatResponse) {
	if info == nil || !HasGeminiResponseOutput(response) {
		return
	}
	runes := 0
	if response != nil {
		for _, candidate := range response.Candidates {
			for _, part := range candidate.Content.Parts {
				runes = saturatingAdd(runes, utf8.RuneCountInString(part.Text))
			}
		}
	}
	info.MarkDeliverableOutput(runes)
}

func MarkClaudeResponseOutput(info *RelayInfo, response *dto.ClaudeResponse) {
	if info == nil || !HasClaudeResponseOutput(response) {
		return
	}
	runes := 0
	if response != nil {
		for _, message := range response.Content {
			runes = saturatingAdd(runes, utf8.RuneCountInString(message.GetText()))
			runes = saturatingAdd(runes, utf8.RuneCountInString(message.GetStringContent()))
			if message.Thinking != nil {
				runes = saturatingAdd(runes, utf8.RuneCountInString(*message.Thinking))
			}
		}
	}
	info.MarkDeliverableOutput(runes)
}
