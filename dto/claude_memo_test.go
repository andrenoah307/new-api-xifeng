package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessageParseContentFastPathsAndFallback(t *testing.T) {
	text := "typed text"
	typed := &ClaudeMessage{Content: []ClaudeMediaMessage{{Type: ContentTypeText, Text: &text}}}
	typedParts, err := typed.ParseContent()
	require.NoError(t, err)
	require.Len(t, typedParts, 1)
	assert.Equal(t, "typed text", typedParts[0].GetText())
	secondRead, err := typed.ParseContent()
	require.NoError(t, err)
	assert.Equal(t, typedParts, secondRead)

	nilContent := &ClaudeMessage{}
	nilParts, err := nilContent.ParseContent()
	require.NoError(t, err)
	assert.Nil(t, nilParts)

	raw := &ClaudeMessage{Content: []any{map[string]any{
		"type":         "tool_use",
		"text":         "raw text",
		"model":        "claude-test",
		"source":       map[string]any{"type": "url", "media_type": "image/png", "url": "https://example.com/image.png", "data": "data"},
		"stop_reason":  "tool_use",
		"partial_json": "{}",
		"role":         "assistant",
		"thinking":     "thought",
		"signature":    "signature",
		"delta":        "delta",
		"cache_control": map[string]any{
			"type": "ephemeral",
		},
		"id":          "call_1",
		"name":        "lookup",
		"input":       map[string]any{"q": "x"},
		"content":     "result",
		"tool_use_id": "parent_1",
	}}}
	rawParts, err := raw.ParseContent()
	require.NoError(t, err)
	require.Len(t, rawParts, 1)
	part := rawParts[0]
	assert.Equal(t, "tool_use", part.Type)
	assert.Equal(t, "raw text", part.GetText())
	assert.Equal(t, "claude-test", part.Model)
	require.NotNil(t, part.Source)
	assert.Equal(t, "url", part.Source.Type)
	assert.Equal(t, "image/png", part.Source.MediaType)
	assert.Equal(t, "https://example.com/image.png", part.Source.Url)
	assert.Equal(t, "data", part.Source.Data)
	require.NotNil(t, part.StopReason)
	assert.Equal(t, "tool_use", *part.StopReason)
	require.NotNil(t, part.PartialJson)
	assert.Equal(t, "{}", *part.PartialJson)
	assert.Equal(t, "assistant", part.Role)
	require.NotNil(t, part.Thinking)
	assert.Equal(t, "thought", *part.Thinking)
	assert.Equal(t, "signature", part.Signature)
	assert.Equal(t, "delta", part.Delta)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(part.CacheControl))
	assert.Equal(t, "call_1", part.Id)
	assert.Equal(t, "lookup", part.Name)
	assert.Equal(t, map[string]any{"q": "x"}, part.Input)
	assert.Equal(t, "result", part.Content)
	assert.Equal(t, "parent_1", part.ToolUseId)

	fallbackCases := []struct {
		name    string
		content any
		check   func(*testing.T, []ClaudeMediaMessage)
	}{
		{
			name:    "non any slice",
			content: []map[string]any{{"type": "text", "text": "fallback"}},
			check: func(t *testing.T, parts []ClaudeMediaMessage) {
				require.Len(t, parts, 1)
				assert.Equal(t, "fallback", parts[0].GetText())
			},
		},
		{
			name:    "typed item in any slice",
			content: []any{ClaudeMediaMessage{Type: ContentTypeText, Text: &text}},
			check: func(t *testing.T, parts []ClaudeMediaMessage) {
				require.Len(t, parts, 1)
				assert.Equal(t, "typed text", parts[0].GetText())
			},
		},
		{
			name: "usage map",
			content: []any{map[string]any{
				"type":  "message_start",
				"usage": map[string]any{"input_tokens": 12},
			}},
			check: func(t *testing.T, parts []ClaudeMediaMessage) {
				require.Len(t, parts, 1)
				require.NotNil(t, parts[0].Usage)
				assert.Equal(t, 12, parts[0].Usage.InputTokens)
			},
		},
	}
	for _, test := range fallbackCases {
		t.Run(test.name, func(t *testing.T) {
			message := &ClaudeMessage{Content: test.content}
			parts, err := message.ParseContent()
			require.NoError(t, err)
			test.check(t, parts)
		})
	}

	invalidCases := []struct {
		name    string
		content any
	}{
		{name: "invalid string field", content: []any{map[string]any{"type": 1}}},
		{name: "invalid pointer string field", content: []any{map[string]any{"text": 1}}},
		{name: "invalid source", content: []any{map[string]any{"source": "bad"}}},
		{name: "invalid source string field", content: []any{map[string]any{"source": map[string]any{"type": 1}}}},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			message := &ClaudeMessage{Content: test.content}
			parts, err := message.ParseContent()
			require.Error(t, err)
			assert.Nil(t, parts)
		})
	}
}

func TestClaudeMessageContentSettersInvalidateMemo(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ClaudeMessage)
		check  func(*testing.T, *ClaudeMessage)
	}{
		{
			name: "SetStringContent",
			mutate: func(message *ClaudeMessage) {
				message.SetStringContent("new string")
			},
			check: func(t *testing.T, message *ClaudeMessage) {
				parsed, err := message.ParseContent()
				require.Error(t, err)
				assert.Nil(t, parsed)
				assert.Equal(t, "new string", message.GetStringContent())
			},
		},
		{
			name: "SetContent",
			mutate: func(message *ClaudeMessage) {
				newText := "new array"
				message.SetContent([]ClaudeMediaMessage{{Type: ContentTypeText, Text: &newText}})
			},
			check: func(t *testing.T, message *ClaudeMessage) {
				parsed, err := message.ParseContent()
				require.NoError(t, err)
				require.Len(t, parsed, 1)
				assert.Equal(t, "new array", parsed[0].GetText())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldText := "old"
			message := &ClaudeMessage{
				Role:    "user",
				Content: []ClaudeMediaMessage{{Type: ContentTypeText, Text: &oldText}},
			}

			parsed, err := message.ParseContent()
			require.NoError(t, err)
			require.Len(t, parsed, 1)
			require.Equal(t, "old", parsed[0].GetText())

			test.mutate(message)
			test.check(t, message)
		})
	}
}

func TestClaudeRequestSystemSettersInvalidateMemo(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ClaudeRequest)
		check  func(*testing.T, *ClaudeRequest)
	}{
		{
			name: "SetStringSystem",
			mutate: func(request *ClaudeRequest) {
				request.SetStringSystem("new string system")
			},
			check: func(t *testing.T, request *ClaudeRequest) {
				assert.Equal(t, "new string system", request.GetStringSystem())
				assert.Empty(t, request.ParseSystem())
			},
		},
		{
			name: "SetSystem",
			mutate: func(request *ClaudeRequest) {
				newText := "new array system"
				request.SetSystem([]ClaudeMediaMessage{{Type: ContentTypeText, Text: &newText}})
			},
			check: func(t *testing.T, request *ClaudeRequest) {
				parsed := request.ParseSystem()
				require.Len(t, parsed, 1)
				assert.Equal(t, "new array system", parsed[0].GetText())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldText := "old system"
			request := &ClaudeRequest{
				System: []ClaudeMediaMessage{{Type: ContentTypeText, Text: &oldText}},
			}

			parsed := request.ParseSystem()
			require.Len(t, parsed, 1)
			require.Equal(t, "old system", parsed[0].GetText())

			test.mutate(request)
			test.check(t, request)
		})
	}
}

func TestClaudeRequestShallowCopyDoesNotReuseSystemMemo(t *testing.T) {
	oldText := "old system"
	newText := "injected system"
	original := &ClaudeRequest{
		System: []ClaudeMediaMessage{{Type: ContentTypeText, Text: &oldText}},
	}
	require.Equal(t, "old system", original.ParseSystem()[0].GetText())

	copied := *original
	// This is the shallow-copy reset used by the relay path. The subsequent
	// direct assignment proves that the copied request no longer inherited the
	// original request's parsed view.
	copied.SetSystem(copied.System)
	copied.System = []ClaudeMediaMessage{{Type: ContentTypeText, Text: &newText}}

	parsedCopy := copied.ParseSystem()
	require.Len(t, parsedCopy, 1)
	assert.Equal(t, "injected system", parsedCopy[0].GetText())
	parsedOriginal := original.ParseSystem()
	require.Len(t, parsedOriginal, 1)
	assert.Equal(t, "old system", parsedOriginal[0].GetText())
}

func TestClaudeRequestTokenCountMetaUsesMutatedStructuredSystem(t *testing.T) {
	oldText := "old system"
	newText := "injected system"
	request := &ClaudeRequest{
		System: []ClaudeMediaMessage{{Type: ContentTypeText, Text: &oldText}},
		Messages: []ClaudeMessage{{
			Role:    "user",
			Content: "hello",
		}},
	}
	require.Equal(t, "old system", request.ParseSystem()[0].GetText())

	request.SetSystem([]ClaudeMediaMessage{{Type: ContentTypeText, Text: &newText}})

	meta := request.GetTokenCountMeta()
	assert.Equal(t, "injected system\nuser\nhello", meta.CombineText)
}

func TestClaudeRequestTokenCountAndToolLookupCompatibility(t *testing.T) {
	const imageData = "aGVsbG8="
	tests := []struct {
		name          string
		body          string
		wantMeta      *types.TokenCountMeta
		wantToolNames map[string]string
	}{
		{
			name: "pure string content",
			body: `{
				"model":"claude-test",
				"system":"plain system",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}]
			}`,
			wantMeta: &types.TokenCountMeta{
				TokenType:     types.TokenTypeTokenizer,
				CombineText:   "plain system\nuser\nhello",
				MessagesCount: 1,
				Files:         make([]*types.FileMeta, 0),
				MaxTokens:     256,
			},
		},
		{
			name: "array content",
			body: `{
				"model":"claude-test",
				"system":[{"type":"text","text":"array system"}],
				"max_tokens":256,
				"messages":[{"role":"user","content":[{"type":"text","text":"array text"}]}]
			}`,
			wantMeta: &types.TokenCountMeta{
				TokenType:     types.TokenTypeTokenizer,
				CombineText:   "array system\nuser\narray text",
				MessagesCount: 1,
				Files:         make([]*types.FileMeta, 0),
				MaxTokens:     256,
			},
		},
		{
			name: "multi turn tool use and results",
			body: `{
				"model":"claude-test",
				"max_tokens":256,
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
			}`,
			wantMeta: &types.TokenCountMeta{
				TokenType: types.TokenTypeTokenizer,
				CombineText: "assistant\nlookup_weather\n{\"city\":\"Paris\"}\ncalculate\n{\"x\":2}\n" +
					"user\n\"sunny\"\n\"4\"",
				MessagesCount: 2,
				Files:         make([]*types.FileMeta, 0),
				MaxTokens:     256,
			},
			wantToolNames: map[string]string{
				"call_weather": "lookup_weather",
				"call_math":    "calculate",
			},
		},
		{
			name: "CJK text",
			body: `{
				"model":"claude-test",
				"system":"系统规则",
				"max_tokens":256,
				"messages":[{"role":"user","content":"你好，世界"}]
			}`,
			wantMeta: &types.TokenCountMeta{
				TokenType:     types.TokenTypeTokenizer,
				CombineText:   "系统规则\nuser\n你好，世界",
				MessagesCount: 1,
				Files:         make([]*types.FileMeta, 0),
				MaxTokens:     256,
			},
		},
		{
			name: "base64 image",
			body: `{
				"model":"claude-test",
				"max_tokens":256,
				"messages":[{"role":"user","content":[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imageData + `"}},
					{"type":"text","text":"describe"}
				]}]
			}`,
			wantMeta: &types.TokenCountMeta{
				TokenType:     types.TokenTypeTokenizer,
				CombineText:   "user\ndescribe",
				MessagesCount: 1,
				Files: []*types.FileMeta{{
					FileType: types.FileTypeImage,
					Source:   types.NewFileSourceFromData(imageData, "image/png"),
				}},
				MaxTokens: 256,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request ClaudeRequest
			require.NoError(t, common.Unmarshal([]byte(test.body), &request))

			assert.Equal(t, test.wantMeta, request.GetTokenCountMeta())
			for toolCallID, wantName := range test.wantToolNames {
				assert.Equal(t, wantName, request.SearchToolNameByToolCallId(toolCallID), toolCallID)
			}
			if test.wantToolNames != nil {
				assert.Equal(t, test.wantToolNames, request.ToolCallNameIndex())
			}
			assert.Empty(t, request.SearchToolNameByToolCallId("missing"))
		})
	}
}

func TestClaudeRequestTokenCountMetaAllContentKinds(t *testing.T) {
	systemText := "system"
	messageText := "answer"
	request := &ClaudeRequest{
		System: []ClaudeMediaMessage{
			{Type: ContentTypeText, Text: &systemText},
			{Type: "image", Source: &ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: "system-image"}},
		},
		Messages: []ClaudeMessage{
			{Role: "user", Content: ""},
			{Role: "assistant", Content: []ClaudeMediaMessage{
				{Type: ContentTypeText, Text: &messageText},
				{Type: "image", Source: &ClaudeMessageSource{Type: "url", MediaType: "image/jpeg", Url: "https://example.com/image.jpg"}},
				{Type: "tool_use", Id: "call_1", Name: "lookup", Input: map[string]any{"q": "x"}},
				{Type: "tool_use"},
				{Type: "tool_result", ToolUseId: "call_1", Content: map[string]any{"ok": true}},
				{Type: "tool_result"},
				{Type: "unknown"},
			}},
		},
		Tools: []any{
			Tool{Name: "normal", Description: "desc", InputSchema: map[string]interface{}{"type": "object"}},
			&Tool{},
			ClaudeWebSearchTool{Name: "web_search", UserLocation: &ClaudeWebSearchUserLocation{Type: "approximate", Timezone: "UTC"}},
			&ClaudeWebSearchTool{},
			"ignored",
		},
	}

	want := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
		CombineText: "system\nuser\nassistant\nanswer\nlookup\n{\"q\":\"x\"}\n{\"ok\":true}\n" +
			"normal\ndesc\n{\"type\":\"object\"}\nweb_search\n{\"type\":\"approximate\",\"timezone\":\"UTC\"}",
		ToolsCount:    4,
		MessagesCount: 2,
		Files: []*types.FileMeta{
			{FileType: types.FileTypeImage, Source: types.NewFileSourceFromData("system-image", "image/png")},
			{FileType: types.FileTypeImage, Source: types.NewFileSourceFromData("https://example.com/image.jpg", "image/jpeg")},
		},
	}
	assert.Equal(t, want, request.GetTokenCountMeta())
}

func TestClaudeRequestTokenCountMetaToolResultMediaUsesFiles(t *testing.T) {
	imageData := "aGVsbG8="
	request := &ClaudeRequest{Messages: []ClaudeMessage{
		{Role: "assistant", Content: []ClaudeMediaMessage{{Type: "tool_use", Id: "call_1", Name: "lookup"}}},
		{Role: "user", Content: []ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call_1", Content: []ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer("before")},
			{Type: "image", Source: &ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: imageData}},
			{Type: "input_text", Text: common.GetPointer("after")},
		}}}},
	}}
	meta := request.GetTokenCountMeta()
	require.Len(t, meta.Files, 1)
	assert.Equal(t, types.FileTypeImage, meta.Files[0].FileType)
	assert.Equal(t, imageData, meta.Files[0].GetRawData())
	assert.Equal(t, "assistant\nlookup\nuser\nbefore\nafter", meta.CombineText)
	assert.NotContains(t, meta.CombineText, imageData)
}

func TestClaudeRequestTokenCountMetaLargeToolResultImageDoesNotBecomeText(t *testing.T) {
	imageData := strings.Repeat("A", 734008)
	request := &ClaudeRequest{Messages: []ClaudeMessage{
		{Role: "assistant", Content: []ClaudeMediaMessage{{Type: "tool_use", Id: "call_1", Name: "lookup"}}},
		{Role: "user", Content: []ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call_1", Content: []ClaudeMediaMessage{{
			Type: "image", Source: &ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: imageData},
		}}}}},
	}}
	meta := request.GetTokenCountMeta()
	require.Len(t, meta.Files, 1)
	assert.Less(t, len(meta.CombineText), 100)
	assert.NotContains(t, meta.CombineText, imageData)
}

func TestClaudeRequestTokenCountMetaToolResultTextAndNilKeepLegacyText(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{name: "string content", content: "result", want: "assistant\nlookup\nuser\n\"result\""},
		{name: "text blocks", content: []ClaudeMediaMessage{{Type: "text", Text: common.GetPointer("result")}, {Type: "input_text", Text: common.GetPointer("second")}}, want: "assistant\nlookup\nuser\n[{\"type\":\"text\",\"text\":\"result\"},{\"type\":\"input_text\",\"text\":\"second\"}]"},
		{name: "nil parse", content: map[string]any{"ok": true}, want: "assistant\nlookup\nuser\n{\"ok\":true}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &ClaudeRequest{Messages: []ClaudeMessage{
				{Role: "assistant", Content: []ClaudeMediaMessage{{Type: "tool_use", Id: "call_1", Name: "lookup"}}},
				{Role: "user", Content: []ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call_1", Content: test.content}}},
			}}
			assert.Equal(t, test.want, request.GetTokenCountMeta().CombineText)
			assert.Empty(t, request.GetTokenCountMeta().Files)
		})
	}
}

func TestClaudeRequestReadHelpersDoNotRetainParsedContent(t *testing.T) {
	var request ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"claude-test",
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":"x"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"ok"}]}
		]
	}`), &request))

	request.GetTokenCountMeta()
	request.SearchToolNameByToolCallId("call_1")

	for i := range request.Messages {
		assert.False(t, request.Messages[i].contentParsed, "message %d retained its parse memo", i)
		assert.Nil(t, request.Messages[i].parsedContent, "message %d retained parsed content", i)
	}
}

func TestClaudeRequestToolCallNameIndexKeepsFirstValidName(t *testing.T) {
	request := &ClaudeRequest{Messages: []ClaudeMessage{
		{Role: "user", Content: "plain text"},
		{Role: "assistant", Content: []ClaudeMediaMessage{
			{Type: "tool_use"},
			{Type: "tool_use", Id: "duplicate", Name: "first"},
			{Type: "tool_use", Id: "duplicate", Name: "second"},
		}},
	}}

	assert.Equal(t, map[string]string{"duplicate": "first"}, request.ToolCallNameIndex())
	for i := range request.Messages {
		assert.False(t, request.Messages[i].contentParsed, "message %d retained its parse memo", i)
		assert.Nil(t, request.Messages[i].parsedContent, "message %d retained parsed content", i)
	}
}

func TestClaudeRequestGetEffortsUsesCommonJSONDecoder(t *testing.T) {
	request := &ClaudeRequest{OutputConfig: json.RawMessage(`{"effort":"high"}`)}
	assert.Equal(t, "high", request.GetEfforts())

	request.OutputConfig = json.RawMessage(`{`)
	assert.Empty(t, request.GetEfforts())
}
