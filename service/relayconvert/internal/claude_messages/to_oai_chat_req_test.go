package claudemessages

import (
	"encoding/json"
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
}
