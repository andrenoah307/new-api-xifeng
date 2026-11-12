package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicErrorRedaction(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		status      int
		want        string
	}{
		{"group zh", `{"error":{"message":"分组 supplier-private 下模型 demo 无可用渠道","code":"model_not_found"}}`, 503, "上游服务暂不可用，请稍后重试"},
		{"group en", `{"error":{"message":"No available channel under group supplier-private"}}`, 400, "请求参数有误，请检查请求内容"},
		{"account", `{"error":{"message":"account supplier-private disabled"}}`, 403, "请求参数有误，请检查请求内容"},
		{"credentials", `{"error":{"message":"Authorization: Bearer supplier-private"}}`, 429, "请求参数有误，请检查请求内容"},
		{"metadata", `{"error":{"message":"failed","metadata":{"provider_name":"supplier-private"}}}`, 422, "请求参数有误，请检查请求内容"},
		{"code object", `{"error":{"message":"failed","code":{"channel_id":"supplier-private"}}}`, 503, "上游服务暂不可用，请稍后重试"},
		{"sibling secret", `{"error":{"message":"group unavailable"},"token":"supplier-private"}`, 503, "上游服务暂不可用，请稍后重试"},
		{"outer response secret", `{"type":"response.failed","response":{"error":{"message":"group unavailable"}},"metadata":{"key":"supplier-private"}}`, 503, "上游服务暂不可用，请稍后重试"},
		{"escaped", `{"error":{"message":"\u5206\u7ec4 supplier-private unavailable"}}`, 503, "上游服务暂不可用，请稍后重试"},
		{"stream", `{"type":"error","message":"channel supplier-private failed","sequence_number":9007199254740993}`, 0, "上游服务暂不可用，请稍后重试"},
		{"responses", `{"type":"response.failed","response":{"id":"resp-local","error":{"message":"channel supplier-private failed","status_code":429},"usage":{"input_tokens":17}}}`, 0, "请求参数有误，请检查请求内容"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, changed := RedactPublicErrorJSON([]byte(tc.input), tc.status, "local-request")
			require.True(t, changed)
			assert.Contains(t, string(out), tc.want)
			assert.Contains(t, string(out), "local-request")
			assert.NotContains(t, string(out), "supplier-private")
			if tc.name == "responses" {
				assert.Contains(t, string(out), `"input_tokens":17`)
			}
			if tc.name == "stream" {
				assert.Contains(t, string(out), "9007199254740993")
			}
		})
	}
	for _, input := range []string{
		`{"error":{"message":"max_tokens must be positive","code":"invalid_request_error","param":"max_tokens"}}`,
		`{"error":{"message":"group: Extra inputs are not permitted"}}`,
		`{"choices":[{"delta":{"content":"分组 supplier-private"}}],"usage":{"total_tokens":9007199254740993}}`,
		`{"type":"response.output_text.delta","delta":"channel supplier-private"}`,
		`[DONE]`,
	} {
		out, changed := RedactPublicErrorJSON([]byte(input), 0, "local-request")
		assert.False(t, changed, input)
		assert.Equal(t, input, string(out))
	}
}

func TestUpstreamErrorMessagesValidation(t *testing.T) {
	old := GetUpstreamErrorMessages()
	t.Cleanup(func() { upstreamErrorMessages.Store(&old) })
	require.NoError(t, UpdateUpstreamErrorMessages(`{"server":"Maintenance window"}`))
	assert.Equal(t, "Maintenance window", PublicErrorMessage(503, ""))
	assert.Equal(t, "请求参数有误，请检查请求内容", PublicErrorMessage(422, ""))
	for _, input := range []string{`null`, `[]`, `{"server":"{{upstream}}"}`, `{"server":"line\nbreak"}`, `{"unknown":"value"}`} {
		assert.Error(t, UpdateUpstreamErrorMessages(input), input)
		assert.Equal(t, "Maintenance window", PublicErrorMessage(503, ""))
	}
}
