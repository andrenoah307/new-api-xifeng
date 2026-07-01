package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
