package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}

// TestMarshalUnicodeLineSeparator 回归:goccy/go-json v0.10.2 的 compactString
// 在转义 json.RawMessage 中的 U+2028/U+2029(且同时含需 HTML 转义的字符)时
// 触发 slice bounds out of range panic(上游 issue #507，v0.10.3 的 #479 修复）。
// 生产事故：relay 请求体带这类字符 → Marshal panic → 预扣费被吞。
func TestMarshalUnicodeLineSeparator(t *testing.T) {
	type payload struct {
		Message json.RawMessage `json:"message"`
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "U+2028 with html escape char", raw: "{\"text\":\"Fail \u2028 <\"}"},
		{name: "U+2029 with html escape char", raw: "{\"text\":\"Fail \u2029 <\"}"},
		{name: "U+2028 and U+2029 mixed", raw: "{\"text\":\"a\u2028b\u2029c < > &\"}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := payload{Message: json.RawMessage(tt.raw)}

			var out []byte
			require.NotPanics(t, func() {
				var err error
				out, err = Marshal(in)
				require.NoError(t, err)
			})

			var back payload
			require.NoError(t, Unmarshal(out, &back))

			var wantObj, gotObj map[string]any
			require.NoError(t, Unmarshal([]byte(tt.raw), &wantObj))
			require.NoError(t, Unmarshal(back.Message, &gotObj))
			require.Equal(t, wantObj, gotObj)
		})
	}
}
