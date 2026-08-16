package user_model_rpm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInspectResultStatesAndSortOrder(t *testing.T) {
	tests := []struct {
		name   string
		raw    []interface{}
		status string
		items  []ModelRPM
		err    bool
	}{
		{name: "empty", raw: nil, status: "empty", items: []ModelRPM{}},
		{name: "overflow", raw: []interface{}{"overflow"}, status: "overflow", items: []ModelRPM{}},
		{
			name:   "available",
			raw:    []interface{}{"z-model", int64(2), "a-model", int64(2), "low", int64(1)},
			status: "available",
			items:  []ModelRPM{{Model: "a-model", RPM: 2}, {Model: "z-model", RPM: 2}, {Model: "low", RPM: 1}},
		},
		{
			name:   "model-named-overflow",
			raw:    []interface{}{"overflow", int64(1)},
			status: "available",
			items:  []ModelRPM{{Model: "overflow", RPM: 1}},
		},
		{name: "malformed", raw: []interface{}{"model"}, status: "unavailable", err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, status, err := parseInspectResult(tt.raw)
			assert.Equal(t, tt.status, status)
			if tt.err {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.items, items)
		})
	}
}

func TestUserModelRPMKeyAndMemberEncoding(t *testing.T) {
	assert.Equal(t, "urpm:v1:42", modelRPMKey(42))
	// A model name that itself contains the separator must not corrupt the
	// split: inspect keeps everything after the FIRST separator.
	assert.Equal(t, "request\x1fmodel\x1fvariant", memberFor("request", "model\x1fvariant"))
}
