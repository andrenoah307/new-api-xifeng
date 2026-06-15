package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestOpenaiResponseHasContentDetectsText(t *testing.T) {
	resp := &dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{
			{Message: dto.Message{Role: "assistant", Content: "hello"}},
		},
	}
	require.True(t, openaiResponseHasContent(resp))
}

func TestOpenaiResponseHasContentDetectsReasoning(t *testing.T) {
	reasoning := "let me think"
	resp := &dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{
			{Message: dto.Message{Role: "assistant", Content: "", ReasoningContent: &reasoning}},
		},
	}
	require.True(t, openaiResponseHasContent(resp))
}

func TestOpenaiResponseHasContentDetectsToolCalls(t *testing.T) {
	resp := &dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{
			{Message: dto.Message{Role: "assistant", Content: "", ToolCalls: []byte(`[{"id":"call_1"}]`)}},
		},
	}
	require.True(t, openaiResponseHasContent(resp))
}

func TestOpenaiResponseHasContentRejectsEmpty(t *testing.T) {
	require.False(t, openaiResponseHasContent(nil))
	require.False(t, openaiResponseHasContent(&dto.OpenAITextResponse{}))
	require.False(t, openaiResponseHasContent(&dto.OpenAITextResponse{
		Choices: []dto.OpenAITextResponseChoice{
			{Message: dto.Message{Role: "assistant", Content: ""}},
		},
	}))
}
