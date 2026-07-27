package common

import (
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestPruneObjectsMatchesLegacyBytes(t *testing.T) {
	escapedText := "中文 \"quote\" \\ path\nline"
	tests := []struct {
		name        string
		input       string
		path        string
		contextJSON string
		value       interface{}
	}{
		{
			name: "recursive AND with nested objects arrays and scalar types",
			input: `{
				"type":"root",
				"meta":{"enabled":true,"count":2,"nothing":null,"empty":{},"empty_array":[]},
				"content":[
					{"type":"drop","enabled":true,"score":3,"text":"中文 \"quote\" \\ path\nline"},
					{"type":"drop","enabled":false,"score":4},
					{"type":"wrapper","children":[
						{"type":"drop","enabled":true,"score":5},
						{"type":"keep","enabled":true,"value":null}
					]}
				]
			}`,
			value: map[string]interface{}{
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": "type", "mode": "full", "value": "drop"},
					map[string]interface{}{"path": "enabled", "mode": "full", "value": true},
				},
			},
		},
		{
			name: "recursive OR with partial matches",
			input: `{"items":[
				{"type":"keep","score":1},
				{"type":"drop","score":1},
				{"type":"keep","score":3},
				{"type":"wrapper","items":[{"type":"keep","score":4}]}
			]}`,
			value: map[string]interface{}{
				"logic": "OR",
				"conditions": []interface{}{
					map[string]interface{}{"path": "type", "mode": "full", "value": "drop"},
					map[string]interface{}{"path": "score", "mode": "gte", "value": 3},
				},
			},
		},
		{
			name: "non recursive target array",
			input: `{"content":[
				{"type":"drop"},
				{"type":"wrapper","children":[{"type":"drop"}]},
				{"type":"keep"}
			],"untouched":{"type":"drop"}}`,
			path: "content",
			value: map[string]interface{}{
				"type":      "drop",
				"recursive": false,
			},
		},
		{
			name:  "matching root is retained while descendants are pruned",
			input: `{"type":"drop","child":{"type":"drop"},"items":[{"type":"drop"},{"type":"keep"}]}`,
			value: "drop",
		},
		{
			name: "gjson array query negative index escaped key and scalar comparisons",
			input: `{"nodes":[
				{
					"literal.dot":"中文 \"quote\" \\ path\nline",
					"parts":[{"kind":"text","text":"删除"},{"kind":"other","text":"保留"}],
					"flag":true,"count":2,"nothing":null
				},
				{"literal.dot":"不匹配","parts":[],"flag":false,"count":0,"nothing":null}
			]}`,
			value: map[string]interface{}{
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": `literal\.dot`, "mode": "full", "value": escapedText},
					map[string]interface{}{"path": `parts.#(kind=="text").text`, "mode": "full", "value": "删除"},
					map[string]interface{}{"path": "parts.-1.kind", "mode": "full", "value": "other"},
					map[string]interface{}{"path": "flag", "mode": "full", "value": true},
					map[string]interface{}{"path": "count", "mode": "gte", "value": 2},
					map[string]interface{}{"path": "nothing", "mode": "full", "value": nil},
				},
			},
		},
		{
			name: "wildcard observes canonical map key order",
			input: `{"items":[
				{"attrs":{"z":"last","a":"first"},"weird*key":"match"},
				{"attrs":{"z":"last","a":"other"},"weird*key":"match"}
			]}`,
			value: map[string]interface{}{
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": "attrs.*", "mode": "full", "value": "first"},
					map[string]interface{}{"path": `weird\*key`, "mode": "full", "value": "match"},
				},
			},
		},
		{
			name:        "condition falls back to context",
			input:       `{"items":[{"type":"drop"},{"type":"keep"}]}`,
			contextJSON: `{"request":{"enabled":true}}`,
			value: map[string]interface{}{
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{"path": "type", "mode": "full", "value": "drop"},
					map[string]interface{}{"path": "request.enabled", "mode": "full", "value": true},
				},
			},
		},
		{
			name:  "empty arrays and objects do not match missing key",
			input: `{"empty_array":[],"empty_object":{},"items":[{}, {"nested":{}}]}`,
			value: map[string]interface{}{
				"conditions": map[string]interface{}{"type": "drop"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy, legacyErr := legacyPruneObjectsForCanary(
				[]byte(test.input), test.path, test.contextJSON, test.value,
			)
			require.NoError(t, legacyErr)

			actual, err := pruneObjects([]byte(test.input), test.path, test.contextJSON, test.value)
			require.NoError(t, err)
			assert.Equal(t, legacy, actual)
		})
	}
}

func TestPruneObjectsSnapshotFallsBackForInvalidUTF8Keys(t *testing.T) {
	dropType := func() interface{} {
		return map[string]interface{}{
			"conditions": map[string]interface{}{"type": "drop"},
		}
	}

	buildObject := func(key []byte, value []byte) []byte {
		input := []byte(`{"`)
		input = append(input, key...)
		input = append(input, []byte(`":`)...)
		input = append(input, value...)
		input = append(input, '}')
		return input
	}

	assertMatchesOracle := func(t *testing.T, input []byte) {
		t.Helper()
		value := dropType()
		expected, err := legacyPruneObjectsForCanary(append([]byte(nil), input...), "", "", value)
		require.NoError(t, err)

		var actual []byte
		var actualErr error
		require.NotPanics(t, func() {
			actual, actualErr = pruneObjects(append([]byte(nil), input...), "", "", value)
		})
		require.NoError(t, actualErr)
		assert.Equal(t, expected, actual)
	}

	t.Run("invalid UTF-8 key with array value", func(t *testing.T) {
		assertMatchesOracle(t, buildObject([]byte{0xff}, []byte(`[{"type":"drop"},{"type":"keep"}]`)))
	})

	t.Run("invalid UTF-8 key with object value", func(t *testing.T) {
		assertMatchesOracle(t, buildObject([]byte{0xff}, []byte(`{"type":"drop","nested":{"type":"keep"}}`)))
	})

	t.Run("truncated multi-byte key with array value", func(t *testing.T) {
		assertMatchesOracle(t, buildObject([]byte{0xe4, 0xb8}, []byte(`[{"type":"drop"},{"type":"keep"}]`)))
	})

	t.Run("invalid key nested deeply", func(t *testing.T) {
		input := []byte(`{"outer":{"inner":{"`)
		input = append(input, 0xff)
		input = append(input, []byte(`":[{"type":"drop"},{"type":"keep"}]}}}`)...)
		assertMatchesOracle(t, input)
	})

	t.Run("normal and mismatched keys share one tree", func(t *testing.T) {
		input := []byte(`{"normal":[{"type":"drop"},{"type":"keep"}],"`)
		input = append(input, 0xff)
		input = append(input, []byte(`":[{"type":"drop"},{"type":"keep"}]}`)...)
		assertMatchesOracle(t, input)
	})

	t.Run("replacement character collision falls back for the whole map", func(t *testing.T) {
		input := []byte(`{"`)
		input = append(input, []byte{0xef, 0xbf, 0xbd}...)
		input = append(input, []byte(`":{"type":"keep"},"`)...)
		input = append(input, 0xff)
		input = append(input, []byte(`":{"type":"drop"}}`)...)
		assertMatchesOracle(t, input)
	})
}

func TestPruneObjectsSnapshotAlignmentSignalsFallback(t *testing.T) {
	options := pruneObjectsOptions{
		conditions: []ConditionOperation{{Path: "type", Mode: "full", Value: "drop"}},
		logic:      "AND",
		recursive:  true,
	}

	t.Run("empty raw", func(t *testing.T) {
		node := map[string]interface{}{"type": "drop"}
		result, dropped, _, err := pruneObjectsNodeWithSnapshot(node, []byte(`{}`), gjson.Result{}, options, "", false)
		require.NoError(t, err)
		require.True(t, dropped)
		assert.Nil(t, result)
	})

	t.Run("out of bounds and negative index", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			index int
		}{
			{name: "past end", index: 3},
			{name: "negative", index: -1},
		} {
			t.Run(test.name, func(t *testing.T) {
				node := map[string]interface{}{"type": "drop"}
				nodeJSON := gjson.Result{Type: gjson.JSON, Raw: `{}`, Index: test.index}
				result, dropped, _, err := pruneObjectsNodeWithSnapshot(node, []byte(`{}`), nodeJSON, options, "", false)
				require.NoError(t, err)
				require.True(t, dropped)
				assert.Nil(t, result)
			})
		}
	})

	t.Run("raw extends beyond snapshot", func(t *testing.T) {
		node := map[string]interface{}{"type": "drop"}
		nodeJSON := gjson.Result{Type: gjson.JSON, Raw: `{"type":"drop"}`, Index: 0}
		result, dropped, _, err := pruneObjectsNodeWithSnapshot(node, []byte(`{}`), nodeJSON, options, "", false)
		require.NoError(t, err)
		require.True(t, dropped)
		assert.Nil(t, result)
	})

	t.Run("map and array type mismatch", func(t *testing.T) {
		mapNode := map[string]interface{}{"type": "drop"}
		mapResult, mapDropped, _, err := pruneObjectsNodeWithSnapshot(
			mapNode,
			[]byte(`[]`),
			gjson.ParseBytes([]byte(`[]`)),
			options,
			"",
			false,
		)
		require.NoError(t, err)
		require.True(t, mapDropped)
		assert.Nil(t, mapResult)

		arrayNode := []interface{}{
			map[string]interface{}{"type": "drop"},
			map[string]interface{}{"type": "keep"},
		}
		arrayResult, arrayDropped, _, err := pruneObjectsNodeWithSnapshot(
			arrayNode,
			[]byte(`{}`),
			gjson.ParseBytes([]byte(`{}`)),
			options,
			"",
			true,
		)
		require.NoError(t, err)
		require.False(t, arrayDropped)
		actual, ok := arrayResult.([]interface{})
		require.True(t, ok)
		require.Len(t, actual, 1)
		assert.Equal(t, "keep", actual[0].(map[string]interface{})["type"])
	})

	t.Run("array length mismatch", func(t *testing.T) {
		arrayNode := []interface{}{
			map[string]interface{}{"type": "drop"},
			map[string]interface{}{"type": "keep"},
		}
		result, dropped, _, err := pruneObjectsNodeWithSnapshot(
			arrayNode,
			[]byte(`[{}]`),
			gjson.ParseBytes([]byte(`[{}]`)),
			options,
			"",
			true,
		)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.([]interface{})
		require.True(t, ok)
		require.Len(t, actual, 1)
		assert.Equal(t, "keep", actual[0].(map[string]interface{})["type"])
	})
}

func TestPruneObjectsFallbackSubtreeContracts(t *testing.T) {
	dropOptions := pruneObjectsOptions{
		conditions: []ConditionOperation{{Path: "type", Mode: "full", Value: "drop"}},
		logic:      "AND",
		recursive:  true,
	}

	t.Run("fallback recursively preserves R4-b array reuse and replacements", func(t *testing.T) {
		node := map[string]interface{}{
			"drop": map[string]interface{}{"type": "drop"},
			"nested": []interface{}{
				map[string]interface{}{"type": "drop"},
				map[string]interface{}{"type": "keep"},
			},
			"unchanged": []interface{}{
				map[string]interface{}{"type": "keep"},
			},
		}
		// The object-length mismatch forces the whole map into recursive fallback.
		result, dropped, _, err := pruneObjectsNodeWithSnapshot(
			node,
			[]byte(`{"other":{}}`),
			gjson.ParseBytes([]byte(`{"other":{}}`)),
			dropOptions,
			"",
			true,
		)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, actual, "drop")
		nested, ok := actual["nested"].([]interface{})
		require.True(t, ok)
		require.Len(t, nested, 1)
		assert.Equal(t, "keep", nested[0].(map[string]interface{})["type"])
		unchanged, ok := actual["unchanged"].([]interface{})
		require.True(t, ok)
		require.Len(t, unchanged, 1)
	})

	t.Run("fallback honors non-recursive root", func(t *testing.T) {
		node := map[string]interface{}{
			"type":  "keep",
			"child": map[string]interface{}{"type": "drop"},
		}
		nonRecursive := dropOptions
		nonRecursive.recursive = false
		result, dropped, _, err := pruneObjectsNodeWithSnapshot(
			node,
			[]byte(`[]`),
			gjson.ParseBytes([]byte(`[]`)),
			nonRecursive,
			"",
			true,
		)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, actual, "child")
	})

	t.Run("fallback propagates marshal and condition errors", func(t *testing.T) {
		_, _, _, err := pruneObjectsNodeFallback(
			map[string]interface{}{"bad": make(chan int)},
			dropOptions,
			"",
			true,
		)
		require.Error(t, err)

		invalidOptions := pruneObjectsOptions{
			conditions: []ConditionOperation{{Path: "type", Mode: "unsupported", Value: "drop"}},
			logic:      "AND",
			recursive:  true,
		}
		_, _, _, err = pruneObjectsNodeFallback(
			[]interface{}{map[string]interface{}{"type": "keep"}},
			invalidOptions,
			"",
			true,
		)
		require.Error(t, err)
	})
}

func TestPruneObjectsNodeReusesArrayBackingWhenUnchanged(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{
			"type": "keep",
			"children": []interface{}{
				map[string]interface{}{"type": "keep"},
			},
		},
		"scalar",
	}
	options := pruneObjectsOptions{
		conditions: []ConditionOperation{{Path: "type", Mode: "full", Value: "drop"}},
		logic:      "AND",
		recursive:  true,
	}

	result, dropped, err := pruneObjectsNode(input, options, "", true)
	require.NoError(t, err)
	require.False(t, dropped)
	actual, ok := result.([]interface{})
	require.True(t, ok)
	require.Len(t, actual, len(input))
	assert.Same(t, &input[0], &actual[0])
}

func TestPruneObjectsNodeArrayChanges(t *testing.T) {
	options := pruneObjectsOptions{
		conditions: []ConditionOperation{{Path: "type", Mode: "full", Value: "drop"}},
		logic:      "AND",
		recursive:  true,
	}

	t.Run("dropped element", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{"type": "keep", "id": float64(1)},
			map[string]interface{}{"type": "drop", "id": float64(2)},
			map[string]interface{}{"type": "keep", "id": float64(3)},
		}

		result, dropped, err := pruneObjectsNode(input, options, "", true)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.([]interface{})
		require.True(t, ok)
		require.Len(t, actual, 2)
		assert.Equal(t, float64(1), actual[0].(map[string]interface{})["id"])
		assert.Equal(t, float64(3), actual[1].(map[string]interface{})["id"])
	})

	t.Run("nested array replacement", func(t *testing.T) {
		input := []interface{}{
			[]interface{}{
				map[string]interface{}{"type": "drop"},
				map[string]interface{}{"type": "keep"},
			},
		}

		result, dropped, err := pruneObjectsNode(input, options, "", true)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.([]interface{})
		require.True(t, ok)
		require.Len(t, actual, 1)
		assert.NotSame(t, &input[0], &actual[0])
		nested, ok := actual[0].([]interface{})
		require.True(t, ok)
		require.Len(t, nested, 1)
		assert.Equal(t, "keep", nested[0].(map[string]interface{})["type"])
	})

	t.Run("in place map mutation does not replace array element", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"type": "keep",
				"children": []interface{}{
					map[string]interface{}{"type": "drop"},
					map[string]interface{}{"type": "keep"},
				},
			},
		}

		result, dropped, err := pruneObjectsNode(input, options, "", true)
		require.NoError(t, err)
		require.False(t, dropped)
		actual, ok := result.([]interface{})
		require.True(t, ok)
		assert.Same(t, &input[0], &actual[0])
		children := actual[0].(map[string]interface{})["children"].([]interface{})
		require.Len(t, children, 1)
		assert.Equal(t, "keep", children[0].(map[string]interface{})["type"])
	})
}

func TestPruneObjectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr string
	}{
		{
			name:    "conditions has invalid type",
			value:   map[string]interface{}{"conditions": "invalid"},
			wantErr: "conditions must be an array or object",
		},
		{
			name: "condition entry is incomplete",
			value: map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{"path": "type"}},
			},
			wantErr: "condition path/mode is required",
		},
		{
			name:    "value has invalid type",
			value:   42,
			wantErr: "prune_objects value must be string or object",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := pruneObjects([]byte(`{"type":"keep"}`), "", "", test.value)
			require.EqualError(t, err, test.wantErr)
			assert.Nil(t, result)
		})
	}
}

func TestPruneObjectsNodeReturnsSnapshotMarshalError(t *testing.T) {
	input := map[string]interface{}{"unsupported": make(chan int)}
	options := pruneObjectsOptions{
		conditions: []ConditionOperation{{Path: "type", Mode: "full", Value: "drop"}},
		logic:      "AND",
		recursive:  true,
	}

	result, dropped, err := pruneObjectsNode(input, options, "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
	assert.Nil(t, result)
	assert.False(t, dropped)
}

// legacyPruneObjectsForCanary preserves the pre-optimization traversal so the
// production implementation can be checked byte-for-byte against it.
func legacyPruneObjectsForCanary(data []byte, path, contextJSON string, value interface{}) ([]byte, error) {
	options, err := parsePruneObjectsOptions(value)
	if err != nil {
		return nil, err
	}

	if path == "" {
		var root interface{}
		if err := common2.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		cleaned, _, err := legacyPruneObjectsNode(root, options, contextJSON, true)
		if err != nil {
			return nil, err
		}
		return common2.Marshal(cleaned)
	}

	target := gjson.GetBytes(data, path)
	if !target.Exists() {
		return data, nil
	}

	var targetNode interface{}
	if target.Type == gjson.JSON {
		if err := common2.UnmarshalJsonStr(target.Raw, &targetNode); err != nil {
			return nil, err
		}
	} else {
		targetNode = target.Value()
	}

	cleaned, _, err := legacyPruneObjectsNode(targetNode, options, contextJSON, true)
	if err != nil {
		return nil, err
	}
	cleanedBytes, err := common2.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(data, path, cleanedBytes)
}

func legacyPruneObjectsNode(node interface{}, options pruneObjectsOptions, contextJSON string, isRoot bool) (interface{}, bool, error) {
	switch value := node.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			next, drop, err := legacyPruneObjectsNode(item, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				continue
			}
			result = append(result, next)
		}
		return result, false, nil
	case map[string]interface{}:
		nodeBytes, err := common2.Marshal(value)
		if err != nil {
			return nil, false, err
		}
		shouldDrop, err := checkConditions(nodeBytes, contextJSON, options.conditions, options.logic)
		if err != nil {
			return nil, false, err
		}
		if shouldDrop && !isRoot {
			return nil, true, nil
		}
		if !options.recursive {
			return value, false, nil
		}
		for key, child := range value {
			next, drop, err := legacyPruneObjectsNode(child, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				delete(value, key)
				continue
			}
			value[key] = next
		}
		return value, false, nil
	default:
		return node, false, nil
	}
}
