package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteStreamOverloadErrorCode(t *testing.T) {
	tests := []struct {
		name             string
		data             string
		wantOriginalCode string
		wantChanged      bool
		wantCodes        map[string]string
	}{
		{
			name:             "server_is_overloaded in response error",
			data:             `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"capacity unavailable"}},"sequence_number":3}`,
			wantOriginalCode: "server_is_overloaded",
			wantChanged:      true,
			wantCodes:        map[string]string{"response.error.code": "server_error"},
		},
		{
			name:             "slow_down in top-level error",
			data:             `{"type":"error","error":{"type":"service_unavailable_error","code":"slow_down","message":"try again"},"sequence_number":2}`,
			wantOriginalCode: "slow_down",
			wantChanged:      true,
			wantCodes:        map[string]string{"error.code": "server_error"},
		},
		{
			name:             "both error paths",
			data:             `{"type":"error","response":{"error":{"code":"slow_down"}},"error":{"code":"SERVER_IS_OVERLOADED"}}`,
			wantOriginalCode: "slow_down",
			wantChanged:      true,
			wantCodes: map[string]string{
				"response.error.code": "server_error",
				"error.code":          "server_error",
			},
		},
		{
			name:             "unrelated error code",
			data:             `{"type":"error","error":{"code":"rate_limit_exceeded"}}`,
			wantOriginalCode: "",
			wantChanged:      false,
			wantCodes:        map[string]string{"error.code": "rate_limit_exceeded"},
		},
		{
			name:             "already rewritten",
			data:             `{"type":"error","error":{"code":"server_error"}}`,
			wantOriginalCode: "",
			wantChanged:      false,
			wantCodes:        map[string]string{"error.code": "server_error"},
		},
		{
			name:             "empty payload",
			data:             "",
			wantOriginalCode: "",
			wantChanged:      false,
		},
		{
			name:             "invalid JSON",
			data:             `{"type":"error"`,
			wantOriginalCode: "",
			wantChanged:      false,
		},
		{
			name:             "ordinary response event",
			data:             `{"type":"response.output_text.delta","delta":"hello","sequence_number":4}`,
			wantOriginalCode: "",
			wantChanged:      false,
		},
		{
			name:             "case-insensitive source code",
			data:             `{"type":"error","error":{"code":"SERVER_IS_OVERLOADED"}}`,
			wantOriginalCode: "SERVER_IS_OVERLOADED",
			wantChanged:      true,
			wantCodes:        map[string]string{"error.code": "server_error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patched, originalCode, changed := RewriteStreamOverloadErrorCode(tt.data)

			assert.Equal(t, tt.wantOriginalCode, originalCode)
			assert.Equal(t, tt.wantChanged, changed)
			if !tt.wantChanged {
				assert.Equal(t, tt.data, patched)
			}
			for path, wantCode := range tt.wantCodes {
				assert.Equal(t, wantCode, gjson.Get(patched, path).String(), path)
			}
		})
	}
}

func TestRewriteStreamOverloadErrorCodePreservesUnmodeledFields(t *testing.T) {
	data := `{"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Capacity is temporarily unavailable; retry later."},"sequence_number":2,"unknown":{"nested":"kept"}}`

	patched, originalCode, changed := RewriteStreamOverloadErrorCode(data)

	require.True(t, changed)
	assert.Equal(t, "server_is_overloaded", originalCode)
	assert.Equal(t, "server_error", gjson.Get(patched, "error.code").String())
	assert.Equal(t, int64(2), gjson.Get(patched, "sequence_number").Int())
	assert.Equal(t, "service_unavailable_error", gjson.Get(patched, "error.type").String())
	assert.Equal(t, "Capacity is temporarily unavailable; retry later.", gjson.Get(patched, "error.message").String())
	assert.Equal(t, "kept", gjson.Get(patched, "unknown.nested").String())
}
