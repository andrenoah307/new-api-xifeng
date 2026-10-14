package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiToolHistoryRoundTrip(t *testing.T) {
	for _, snake := range []bool{false, true} {
		name := "camelCase"
		call, response, signature := "functionCall", "functionResponse", "thoughtSignature"
		if snake {
			name, call, response, signature = "snake_case", "function_call", "function_response", "thought_signature"
		}
		t.Run(name, func(t *testing.T) {
			raw := `{"contents":[{"role":"model","parts":[{"` + call + `":{"id":"call-a","name":"lookup","args":{"city":"Paris"}},"` + signature + `":"opaque+/signature=="},{"` + call + `":{"id":"call-b","name":"lookup","args":{"city":"London"}}}]},{"role":"user","parts":[{"` + response + `":{"id":"call-a","name":"lookup","response":{"value":1}}},{"` + response + `":{"id":"call-b","name":"lookup","response":{"value":2}}}]}]}`
			var request GeminiChatRequest
			require.NoError(t, common.UnmarshalJsonStr(raw, &request))
			encoded, err := common.Marshal(request)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, common.Unmarshal(encoded, &wire))
			contents := wire["contents"].([]any)
			parts := contents[0].(map[string]any)["parts"].([]any)
			first := parts[0].(map[string]any)
			require.Contains(t, first, "functionCall")
			assert.Equal(t, "call-a", first["functionCall"].(map[string]any)["id"])
			assert.Equal(t, "opaque+/signature==", first["thoughtSignature"])
			assert.NotContains(t, parts[1].(map[string]any), "thoughtSignature")
			assert.Equal(t, "call-b", parts[1].(map[string]any)["functionCall"].(map[string]any)["id"])
			results := contents[1].(map[string]any)["parts"].([]any)
			require.Contains(t, results[0].(map[string]any), "functionResponse")
			assert.Equal(t, "call-a", results[0].(map[string]any)["functionResponse"].(map[string]any)["id"])
			assert.Equal(t, "call-b", results[1].(map[string]any)["functionResponse"].(map[string]any)["id"])
		})
	}
}

func TestGeminiPartialFunctionCallRoundTrip(t *testing.T) {
	raw := `{"id":"call-a","name":"lookup","args":{},"partialArgs":[{"jsonPath":"$.count","numberValue":0,"willContinue":false},{"jsonPath":"$.enabled","boolValue":false},{"jsonPath":"$.query","stringValue":""},{"jsonPath":"$.optional","nullValue":null}],"willContinue":false}`
	var call FunctionCall
	require.NoError(t, common.UnmarshalJsonStr(raw, &call))
	encoded, err := common.Marshal(call)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(encoded))
}
