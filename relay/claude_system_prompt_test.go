package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyClaudeSystemPrompt(t *testing.T) {
	tests := []struct {
		name            string
		system          any
		override        bool
		wantString      string
		wantOverrideSet bool
	}{
		{
			name:       "add to empty system",
			system:     nil,
			wantString: "channel system",
		},
		{
			name:       "preserve existing system without override",
			system:     "client system",
			wantString: "client system",
		},
		{
			name:            "prepend existing string when overriding",
			system:          "  client system  ",
			override:        true,
			wantString:      "channel system\nclient system",
			wantOverrideSet: true,
		},
		{
			name:            "replace blank string when overriding",
			system:          "   ",
			override:        true,
			wantString:      "channel system",
			wantOverrideSet: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(nil)
			request := &dto.ClaudeRequest{System: test.system}

			applyClaudeSystemPrompt(ctx, request, dto.ChannelSettings{
				SystemPrompt:         "channel system",
				SystemPromptOverride: test.override,
			})

			assert.Equal(t, test.wantString, request.GetStringSystem())
			assert.Equal(t, test.wantOverrideSet, common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride))
		})
	}
}

func TestApplyClaudeSystemPromptHandlesEmptyPromptAndStructuredSystem(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	request := &dto.ClaudeRequest{System: "client system"}
	applyClaudeSystemPrompt(ctx, request, dto.ChannelSettings{})
	assert.Equal(t, "client system", request.GetStringSystem())

	request = &dto.ClaudeRequest{System: []dto.ClaudeMediaMessage{}}
	applyClaudeSystemPrompt(ctx, request, dto.ChannelSettings{
		SystemPrompt:         "channel system",
		SystemPromptOverride: true,
	})
	parsed := request.ParseSystem()
	require.Len(t, parsed, 1)
	assert.Equal(t, "channel system", parsed[0].GetText())
}

func TestApplyClaudeSystemPromptInvalidatesStructuredMemoBeforeConversion(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	clientText := "client system"
	original := &dto.ClaudeRequest{
		Model:  "claude-test",
		System: []dto.ClaudeMediaMessage{{Type: dto.ContentTypeText, Text: &clientText}},
		Messages: []dto.ClaudeMessage{{
			Role:    "user",
			Content: "hello",
		}},
	}
	require.Equal(t, "client system", original.ParseSystem()[0].GetText())

	copied := copyClaudeRequestForRelay(original)
	applyClaudeSystemPrompt(ctx, copied, dto.ChannelSettings{
		SystemPrompt:         "channel system",
		SystemPromptOverride: true,
	})

	parsed := copied.ParseSystem()
	require.Len(t, parsed, 2)
	assert.Equal(t, "channel system", parsed[0].GetText())
	assert.Equal(t, "client system", parsed[1].GetText())
	converted, err := service.ClaudeToOpenAIRequest(*copied, nil)
	require.NoError(t, err)
	require.NotEmpty(t, converted.Messages)
	assert.Equal(t, "channel systemclient system", converted.Messages[0].StringContent())
	assert.Equal(t, "client system", original.ParseSystem()[0].GetText())
}
