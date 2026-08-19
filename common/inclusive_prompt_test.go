package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInclusivePromptChannelIDs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[int]struct{}
	}{
		{
			name: "empty",
			raw:  "",
			want: map[int]struct{}{},
		},
		{
			name: "valid channel ids",
			raw:  "300,289",
			want: map[int]struct{}{
				289: {},
				300: {},
			},
		},
		{
			name: "whitespace and invalid items",
			raw:  " 300 , , abc, -1 , 289",
			want: map[int]struct{}{
				289: {},
				300: {},
			},
		},
		{
			name: "all invalid",
			raw:  " , abc, -1, 0",
			want: map[int]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseInclusivePromptChannelIDs(tt.raw))
		})
	}
}
