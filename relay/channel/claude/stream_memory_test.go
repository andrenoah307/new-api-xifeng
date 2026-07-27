package claude

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamRuneCounterMatchesWholeText(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "empty stream", chunks: nil},
		{name: "single rune split", chunks: []string{"a", "b"}},
		{name: "CJK", chunks: []string{"你", "好", "世", "界"}},
		{name: "emoji", chunks: []string{"😀", "🎉"}},
		{name: "odd rune count", chunks: []string{"a", "b", "c", "d", "e"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
			for _, chunk := range tt.chunks {
				text := chunk
				ok := FormatClaudeResponseInfo(&dto.ClaudeResponse{
					Type:  "content_block_delta",
					Delta: &dto.ClaudeMediaMessage{Text: &text},
				}, nil, claudeInfo)
				require.True(t, ok)
			}

			wholeText := strings.Join(tt.chunks, "")
			runeCount := 0
			for _, chunk := range tt.chunks {
				runeCount += utf8.RuneCountInString(chunk)
			}
			assert.Equal(t, runeCount, claudeInfo.ResponseTextRuneCount)
			assert.Equal(t, service.CountTextToken(wholeText, "claude-test"),
				service.EstimateTokenByRuneCount(claudeInfo.ResponseTextRuneCount))
		})
	}
}

func TestClaudeStreamRuneCounterIncludesThinkingDeltas(t *testing.T) {
	thinking := "思考😀"
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	ok := FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type:  "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{Thinking: &thinking},
	}, nil, claudeInfo)

	require.True(t, ok)
	assert.Equal(t, utf8.RuneCountInString(thinking), claudeInfo.ResponseTextRuneCount)
	assert.Equal(t, service.CountTextToken(thinking, "claude-test"),
		service.EstimateTokenByRuneCount(claudeInfo.ResponseTextRuneCount))
}
