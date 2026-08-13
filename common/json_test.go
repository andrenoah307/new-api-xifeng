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

// goccy/go-json v0.10.2 panicked with "slice bounds out of range [N:N-2]" when
// compacting a raw JSON string that carries a literal U+2028 / U+2029: the
// cursor was advanced before the copy start was recomputed, so start overshot
// cursor by two bytes. Client payloads reach this path through common.Marshal
// whenever a json.RawMessage is re-marshaled on the relay conversion path, so
// the crash was reachable from ordinary user input.
func TestMarshalRawMessageWithLineSeparators(t *testing.T) {
	type payload struct {
		Raw json.RawMessage `json:"raw"`
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "line separator", raw: "{\"text\":\"before after\"}"},
		{name: "paragraph separator", raw: "{\"text\":\"before after\"}"},
		{name: "both, adjacent", raw: "{\"text\":\"  \"}"},
		{name: "trailing separator", raw: "{\"text\":\"tail \"}"},
		{name: "nested array", raw: "[{\"text\":\"a b\"},\"c d\"]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := Marshal(payload{Raw: json.RawMessage(tt.raw)})
			require.NoError(t, err)

			var decoded payload
			require.NoError(t, Unmarshal(encoded, &decoded))

			// The separators must survive the round trip with their original
			// meaning, whether the encoder emitted them raw or as  .
			var want, got any
			require.NoError(t, Unmarshal([]byte(tt.raw), &want))
			require.NoError(t, Unmarshal(decoded.Raw, &got))
			require.Equal(t, want, got)
		})
	}
}
