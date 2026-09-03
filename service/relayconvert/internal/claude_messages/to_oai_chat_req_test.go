package claudemessages

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesRequestToOpenAIChatConvertsOptionsAndContent(t *testing.T) {
	maxTokens := uint(1024)
	topP := 0.9
	topK := 20
	stream := true
	temperature := 0.7
	text := "text block"
	inputText := "input text block"
	toolResultText := "tool result"
	request := dto.ClaudeRequest{
		Model:         "claude-test",
		System:        "system prompt",
		MaxTokens:     &maxTokens,
		StopSequences: []string{"stop"},
		Temperature:   &temperature,
		TopP:          &topP,
		TopK:          &topK,
		Stream:        &stream,
		Tools: []dto.Tool{{
			Name:        "lookup",
			Description: "lookup data",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "plain message"},
			{Role: "user", Content: []dto.ClaudeMediaMessage{
				{Type: "text", Text: &text},
				{Type: "input_text", Text: &inputText},
				{Type: "image", Source: &dto.ClaudeMessageSource{MediaType: "image/png", Data: "aGVsbG8="}},
			}},
			{Role: "assistant", Content: []dto.ClaudeMediaMessage{
				{Type: "tool_use", Id: "call_1", Name: "lookup", Input: map[string]any{"q": "x"}},
			}},
			{Role: "user", Content: []dto.ClaudeMediaMessage{
				{
					Type:      "tool_result",
					Name:      "lookup",
					ToolUseId: "call_1",
					Content:   []dto.ClaudeMediaMessage{{Type: "text", Text: &toolResultText}},
				},
			}},
			{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "unknown"}}},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-test-thinking",
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}

	converted, err := ClaudeMessagesRequestToOpenAIChat(request, info)
	require.NoError(t, err)
	assert.Equal(t, "claude-test-thinking", converted.Model)
	assert.Equal(t, &maxTokens, converted.MaxTokens)
	assert.Equal(t, &topP, converted.TopP)
	assert.Equal(t, &topK, converted.TopK)
	assert.Equal(t, &stream, converted.Stream)
	assert.Equal(t, &temperature, converted.Temperature)
	assert.Equal(t, "stop", converted.Stop)
	require.Len(t, converted.Tools, 1)
	assert.Equal(t, "lookup", converted.Tools[0].Function.Name)
	assert.Equal(t, "lookup data", converted.Tools[0].Function.Description)
	assert.Equal(t, map[string]interface{}{"type": "object"}, converted.Tools[0].Function.Parameters)

	require.Len(t, converted.Messages, 5)
	assert.Equal(t, "system prompt", converted.Messages[0].StringContent())
	assert.Equal(t, "plain message", converted.Messages[1].StringContent())
	media := converted.Messages[2].ParseContent()
	require.Len(t, media, 3)
	assert.Equal(t, "text block", media[0].Text)
	assert.Equal(t, "input text block", media[1].Text)
	require.NotNil(t, media[2].GetImageMedia())
	assert.Equal(t, "data:image/png;base64,aGVsbG8=", media[2].GetImageMedia().Url)
	toolCalls := converted.Messages[3].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.JSONEq(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, "lookup", *converted.Messages[4].Name)
	assert.JSONEq(t, `[{"type":"text","text":"tool result"}]`, converted.Messages[4].StringContent())
}

func TestClaudeMessagesRequestToOpenAIChatConvertsStructuredSystem(t *testing.T) {
	first := "first"
	second := "second"
	request := dto.ClaudeRequest{
		Model: "claude-test",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: &first},
			{Type: "text"},
			{Type: "text", Text: &second},
		},
	}

	converted, err := ClaudeMessagesRequestToOpenAIChat(request, nil)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	assert.Equal(t, "firstsecond", converted.Messages[0].StringContent())
}

func TestClaudeMessagesRequestToOpenAIChatConvertsOpenRouterReasoning(t *testing.T) {
	budget := 2048
	systemText := "cached system"
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:       constant.ChannelTypeOpenRouter,
		UpstreamModelName: "anthropic/claude-test",
	}}
	tests := []struct {
		name          string
		thinking      *dto.Thinking
		wantMaxTokens int
	}{
		{
			name:          "enabled",
			thinking:      &dto.Thinking{Type: "enabled", BudgetTokens: &budget},
			wantMaxTokens: budget,
		},
		{
			name:     "adaptive",
			thinking: &dto.Thinking{Type: "adaptive"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := dto.ClaudeRequest{
				Model:         "claude-test",
				OutputConfig:  json.RawMessage(`{"effort":"high"}`),
				Thinking:      test.thinking,
				StopSequences: []string{"first", "second"},
			}
			if test.name == "enabled" {
				request.System = []dto.ClaudeMediaMessage{{
					Type:         "text",
					Text:         &systemText,
					CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
				}}
			}

			converted, err := ClaudeMessagesRequestToOpenAIChat(request, info)
			require.NoError(t, err)
			var verbosity string
			require.NoError(t, common.Unmarshal(converted.Verbosity, &verbosity))
			assert.Equal(t, "high", verbosity)
			var reasoning openRouterRequestReasoning
			require.NoError(t, common.Unmarshal(converted.Reasoning, &reasoning))
			assert.True(t, reasoning.Enabled)
			assert.Equal(t, test.wantMaxTokens, reasoning.MaxTokens)
			assert.Equal(t, []string{"first", "second"}, converted.Stop)
			if test.name == "enabled" {
				require.Len(t, converted.Messages, 1)
				parts := converted.Messages[0].ParseContent()
				require.Len(t, parts, 1)
				assert.Equal(t, "cached system", parts[0].Text)
				assert.JSONEq(t, `{"type":"ephemeral"}`, string(parts[0].CacheControl))
			}
		})
	}
}

func TestClaudeMessagesRequestToOpenAIChatRejectsInvalidContent(t *testing.T) {
	var request dto.ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"claude-test",
		"messages":[{"role":"user","content":[{"type":1}]}]
	}`), &request))

	converted, err := ClaudeMessagesRequestToOpenAIChat(request, nil)
	require.Error(t, err)
	assert.Nil(t, converted)
}

func TestClaudeMessagesRequestToOpenAIChatResolvesAllToolResultNames(t *testing.T) {
	var request dto.ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"claude-test",
		"messages":[
			{"role":"assistant","content":[
				{"type":"tool_use","id":"call_weather","name":"lookup_weather","input":{"city":"Paris"}},
				{"type":"tool_use","id":"call_math","name":"calculate","input":{"x":2}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"call_weather","content":"sunny"},
				{"type":"tool_result","tool_use_id":"call_math","content":"4"}
			]}
		]
	}`), &request))

	converted, err := ClaudeMessagesRequestToOpenAIChat(request, nil)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	require.NotNil(t, converted.Messages[1].Name)
	require.NotNil(t, converted.Messages[2].Name)
	assert.Equal(t, "lookup_weather", *converted.Messages[1].Name)
	assert.Equal(t, "call_weather", converted.Messages[1].ToolCallId)
	assert.Equal(t, "calculate", *converted.Messages[2].Name)
	assert.Equal(t, "call_math", converted.Messages[2].ToolCallId)
	assert.Equal(t, []string{"assistant", "tool", "tool"}, []string{converted.Messages[0].Role, converted.Messages[1].Role, converted.Messages[2].Role})
}

func TestClaudeMessagesRequestToOpenAIChatToolResultGatePreservesLegacyJSON(t *testing.T) {
	tests := []struct {
		name    string
		content any
	}{
		{name: "string", content: "plain result"},
		{name: "text blocks", content: []dto.ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer("alpha")},
			{Type: "input_text", Text: common.GetPointer("beta")},
		}},
		{name: "unparseable map", content: map[string]any{"result": true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
				{Role: "assistant", Content: []dto.ClaudeMediaMessage{{
					Type: "tool_use", Id: "call_1", Name: "lookup", Input: map[string]any{},
				}}},
				{Role: "user", Content: []dto.ClaudeMediaMessage{{
					Type: "tool_result", ToolUseId: "call_1", Content: test.content,
				}}},
			}}

			converted, err := ClaudeMessagesRequestToOpenAIChat(request, nil)
			require.NoError(t, err)
			got, err := common.Marshal(converted)
			require.NoError(t, err)

			encodedContent, err := common.Marshal(test.content)
			require.NoError(t, err)
			if test.name == "string" {
				encodedContent = []byte(`plain result`)
			} else if test.name == "unparseable map" {
				encodedContent = []byte(`null`)
			}
			expectedMessages := []dto.Message{
				{Role: "assistant", Content: nil, ToolCalls: mustToolCallsJSON(t)},
				{Role: "tool", Content: string(encodedContent), Name: common.GetPointer("lookup"), ToolCallId: "call_1"},
			}
			expected, err := common.Marshal(&dto.GeneralOpenAIRequest{Model: request.Model, Messages: expectedMessages})
			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})
	}
}

func TestClaudeMessagesRequestToOpenAIChatRelocatesToolResultImage(t *testing.T) {
	t.Setenv("CLAUDE_TOOL_RESULT_RELOCATE_MEDIA", "true")
	imageData := strings.Repeat("A", 734008)
	cacheControl := json.RawMessage(`{"type":"ephemeral"}`)
	request := dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
		{Role: "assistant", Content: []dto.ClaudeMediaMessage{{
			Type: "tool_use", Id: "call_1", Name: "lookup", Input: map[string]any{"q": "x"},
		}}},
		{Role: "user", Content: []dto.ClaudeMediaMessage{{
			Type: "tool_result", ToolUseId: "call_1", Content: []dto.ClaudeMediaMessage{
				{Type: "text", Text: common.GetPointer("before")},
				{Type: "image", CacheControl: cacheControl, Source: &dto.ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: imageData}},
				{Type: "input_text", Text: common.GetPointer("after")},
			}},
		}},
	}}

	info := &relaycommon.RelayInfo{}
	converted, err := ClaudeMessagesRequestToOpenAIChat(request, info)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	assert.Equal(t, "assistant", converted.Messages[0].Role)
	assert.Equal(t, "tool", converted.Messages[1].Role)
	assert.Equal(t, "call_1", converted.Messages[1].ToolCallId)
	assert.NotContains(t, converted.Messages[1].StringContent(), imageData)
	assert.NotContains(t, converted.Messages[1].StringContent(), "data:image")
	assert.Equal(t, "beforeafter [tool_result_image_relocated]", converted.Messages[1].StringContent())

	assert.Equal(t, "user", converted.Messages[2].Role)
	parts := converted.Messages[2].ParseContent()
	require.Len(t, parts, 1)
	assert.Equal(t, "data:image/png;base64,"+imageData, parts[0].GetImageMedia().Url)
	assert.JSONEq(t, string(cacheControl), string(parts[0].CacheControl))

	body, err := common.Marshal(converted)
	require.NoError(t, err)
	legacyToolContent, err := common.Marshal([]dto.ClaudeMediaMessage{
		{Type: "text", Text: common.GetPointer("before")},
		{Type: "image", CacheControl: cacheControl, Source: &dto.ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: imageData}},
		{Type: "input_text", Text: common.GetPointer("after")},
	})
	require.NoError(t, err)
	assert.Greater(t, len(legacyToolContent), len(converted.Messages[1].StringContent())*1000)
	legacyMessages := []dto.Message{
		{Role: "assistant", Content: nil, ToolCalls: converted.Messages[0].ToolCalls},
		{Role: "tool", Content: string(legacyToolContent), Name: common.GetPointer("lookup"), ToolCallId: "call_1"},
	}
	legacyBody, err := common.Marshal(&dto.GeneralOpenAIRequest{Model: request.Model, Messages: legacyMessages})
	require.NoError(t, err)
	assert.Less(t, len(body), len(legacyBody), "relocated media should not retain the tool_result JSON payload")
	assert.Equal(t, 1, info.ToolResultImageCount)
	assert.Equal(t, len(imageData), info.ToolResultImageBase64Chars)
	assert.Equal(t, []string{"image/png"}, info.ToolResultMediaTypes)
	assert.False(t, info.ToolResultMediaFallback)
}

func TestClaudeMessagesRequestToOpenAIChatToolResultOrderFollowsAssistantToolCalls(t *testing.T) {
	t.Setenv("CLAUDE_TOOL_RESULT_RELOCATE_MEDIA", "true")
	request := dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
		{Role: "assistant", Content: []dto.ClaudeMediaMessage{
			{Type: "tool_use", Id: "call_1", Name: "first"},
			{Type: "tool_use", Id: "call_2", Name: "second"},
		}},
		{Role: "user", Content: []dto.ClaudeMediaMessage{
			{Type: "tool_result", ToolUseId: "call_1", Content: []dto.ClaudeMediaMessage{{Type: "text", Text: common.GetPointer("one")}, {Type: "image", Source: &dto.ClaudeMessageSource{MediaType: "image/png", Data: "one"}}}},
			{Type: "tool_result", ToolUseId: "call_2", Content: []dto.ClaudeMediaMessage{{Type: "text", Text: common.GetPointer("two")}, {Type: "image", Source: &dto.ClaudeMessageSource{MediaType: "image/png", Data: "two"}}}},
		}},
	}}
	converted, err := ClaudeMessagesRequestToOpenAIChat(request, &relaycommon.RelayInfo{})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 4)
	assert.Equal(t, "assistant", converted.Messages[0].Role)
	assert.Equal(t, []string{"call_1", "call_2"}, []string{converted.Messages[1].ToolCallId, converted.Messages[2].ToolCallId})
	assert.Equal(t, "user", converted.Messages[3].Role)
	parts := converted.Messages[3].ParseContent()
	require.Len(t, parts, 2)
	assert.Equal(t, "data:image/png;base64,one", parts[0].GetImageMedia().Url)
	assert.Equal(t, "data:image/png;base64,two", parts[1].GetImageMedia().Url)
}

func TestClaudeMessagesRequestToOpenAIChatToolResultRelocationSwitch(t *testing.T) {
	imageData := "c2hvcnQ="
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled_%t", enabled), func(t *testing.T) {
			t.Setenv("CLAUDE_TOOL_RESULT_RELOCATE_MEDIA", fmt.Sprintf("%t", enabled))
			request := dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
				{Role: "assistant", Content: []dto.ClaudeMediaMessage{{Type: "tool_use", Id: "call_1", Name: "lookup"}}},
				{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call_1", Content: []dto.ClaudeMediaMessage{{
					Type: "image", Source: &dto.ClaudeMessageSource{MediaType: "image/png", Data: imageData},
				}}}}},
			}}
			info := &relaycommon.RelayInfo{}
			converted, err := ClaudeMessagesRequestToOpenAIChat(request, info)
			require.NoError(t, err)
			if enabled {
				require.Len(t, converted.Messages, 3)
			} else {
				require.Len(t, converted.Messages, 2)
			}
			toolMessage := converted.Messages[1]
			assert.NotContains(t, toolMessage.StringContent(), imageData)
			assert.NotContains(t, toolMessage.StringContent(), "data:image")
			if enabled {
				parts := converted.Messages[2].ParseContent()
				require.Len(t, parts, 1)
				assert.Equal(t, "data:image/png;base64,"+imageData, parts[0].GetImageMedia().Url)
			} else {
				assert.Contains(t, converted.Messages[1].StringContent(), "[tool_result_media_fallback]")
			}
			assert.Equal(t, enabled == false, info.ToolResultMediaFallback)
		})
	}
}

func TestClaudeMessagesRequestToOpenAIChatToolResultDocumentFallsBack(t *testing.T) {
	request := dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
		{Role: "assistant", Content: []dto.ClaudeMediaMessage{{Type: "tool_use", Id: "call_1", Name: "lookup"}}},
		{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call_1", Content: []dto.ClaudeMediaMessage{{
			Type: "document", Source: &dto.ClaudeMessageSource{MediaType: "application/pdf", Data: strings.Repeat("B", 128)},
		}}}}},
	}}
	info := &relaycommon.RelayInfo{}
	converted, err := ClaudeMessagesRequestToOpenAIChat(request, info)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	assert.Contains(t, converted.Messages[1].StringContent(), "[tool_result_media_omitted:document]")
	assert.NotContains(t, converted.Messages[1].StringContent(), strings.Repeat("B", 128))
	assert.True(t, info.ToolResultMediaFallback)
	assert.Equal(t, []string{"application/pdf"}, info.ToolResultMediaTypes)
}

func mustToolCallsJSON(t *testing.T) json.RawMessage {
	t.Helper()
	toolCalls, err := common.Marshal([]dto.ToolCallRequest{{
		ID: "call_1", Type: "function", Function: dto.FunctionRequest{Name: "lookup", Arguments: "{}"},
	}})
	require.NoError(t, err)
	return toolCalls
}
