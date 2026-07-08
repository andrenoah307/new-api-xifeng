package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitQueryValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "single value", raw: "success", want: []string{"success"}},
		{name: "multiple values", raw: "success,pending", want: []string{"success", "pending"}},
		{name: "trims whitespace", raw: " success , pending ", want: []string{"success", "pending"}},
		{name: "drops empty parts", raw: "success,,pending,", want: []string{"success", "pending"}},
		{name: "only separators", raw: ",, ,", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SplitQueryValues(tt.raw))
		})
	}
}
