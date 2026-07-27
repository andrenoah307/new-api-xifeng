package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuneCounterEstimateMatchesWholeTextEstimate(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "empty stream", chunks: nil},
		{name: "single runes split across chunks", chunks: []string{"a", "b"}},
		{name: "CJK chunks", chunks: []string{"你", "好", "世", "界"}},
		{name: "emoji chunks", chunks: []string{"😀", "🎉", "🚀"}},
		{name: "odd rune count", chunks: []string{"a", "b", "c", "d", "e"}},
		{name: "mixed unicode chunks", chunks: []string{"Hi ", "你", "好😀", "!"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wholeText := strings.Join(tt.chunks, "")
			runeCount := 0
			for _, chunk := range tt.chunks {
				runeCount += utf8.RuneCountInString(chunk)
			}

			want := CountTextToken(wholeText, "test-model")
			got := EstimateTokenByRuneCount(runeCount)
			require.Equal(t, want, got)
			assert.Equal(t, utf8.RuneCountInString(wholeText), runeCount)
		})
	}
}

func TestRuneCounterEstimateAppliesIntegerRoundingOnce(t *testing.T) {
	chunks := []string{"a", "b"}
	runeCount := 0
	for _, chunk := range chunks {
		runeCount += utf8.RuneCountInString(chunk)
	}

	assert.Equal(t, 3, CountTextToken(strings.Join(chunks, ""), "test-model"))
	assert.Equal(t, 3, EstimateTokenByRuneCount(runeCount))
	assert.NotEqual(t, 2, EstimateTokenByRuneCount(runeCount), "per-chunk 1*3/2 rounding must not be summed")
}
