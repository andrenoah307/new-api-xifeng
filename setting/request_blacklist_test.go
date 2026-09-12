package setting

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func marshalRequestBlacklistConfig(t *testing.T, config RequestBlacklistConfig) string {
	t.Helper()
	data, err := common.Marshal(config)
	require.NoError(t, err)
	return string(data)
}

func preserveRequestBlacklist(t *testing.T) {
	t.Helper()
	previous := RequestBlacklist2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateRequestBlacklistByJSONString(previous))
	})
}

func TestCheckRequestBlacklistRejectsInvalidDocuments(t *testing.T) {
	tooManyWords := make([]string, 5001)
	for i := range tooManyWords {
		tooManyWords[i] = fmt.Sprintf("word-%d", i)
	}
	tooManyGroups := make([]string, 201)
	for i := range tooManyGroups {
		tooManyGroups[i] = fmt.Sprintf("group-%d", i)
	}
	tooManyDomains := make([]string, 5001)
	for i := range tooManyDomains {
		tooManyDomains[i] = fmt.Sprintf("host-%d.example", i)
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "non object", raw: `[]`},
		{name: "null object", raw: `null`},
		{name: "unknown field", raw: `{"mode":"off","unknown":true}`},
		{name: "wrong field type", raw: `{"mode":7}`},
		{name: "null field", raw: `{"domains":null}`},
		{name: "invalid mode", raw: `{"mode":"block"}`},
		{name: "status 500", raw: `{"block_status_code":500}`},
		{name: "status 200", raw: `{"block_status_code":200}`},
		{name: "negative status", raw: `{"block_status_code":-1}`},
		{name: "word too long", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{ContentWords: []string{strings.Repeat("界", 129)}})},
		{name: "too many words", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{ContentWords: tooManyWords})},
		{name: "too many groups", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{EnabledGroups: tooManyGroups})},
		{name: "too many domains", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{Domains: tooManyDomains})},
		{name: "empty domain", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{Domains: []string{" "}})},
		{name: "domain scheme", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{Domains: []string{"https://example.com"}})},
		{name: "domain path", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{Domains: []string{"example.com/private"}})},
		{name: "domain wildcard", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{Domains: []string{"*.example.com"}})},
		{name: "masked message", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{BlockMessage: "Blocked by example.com"})},
		{name: "message too long", raw: marshalRequestBlacklistConfig(t, RequestBlacklistConfig{BlockMessage: strings.Repeat("界", 501)})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, CheckRequestBlacklist(test.raw))
		})
	}
}

func TestRequestBlacklistNormalizationAndRoundTrip(t *testing.T) {
	preserveRequestBlacklist(t)
	input := RequestBlacklistConfig{
		Mode:            "enforce",
		EnabledGroups:   []string{" group-b ", "group-a", "group-b", ""},
		ContentWords:    []string{" Secret ", "secret", "中文"},
		Domains:         []string{"BÜCHER.example.:443", "xn--bcher-kva.example"},
		BlockMessage:    " 请求内容不符合策略 ",
		BlockStatusCode: 0,
	}
	raw := marshalRequestBlacklistConfig(t, input)

	require.NoError(t, CheckRequestBlacklist(raw))
	require.NoError(t, UpdateRequestBlacklistByJSONString(raw))

	var normalized RequestBlacklistConfig
	require.NoError(t, common.UnmarshalJsonStr(RequestBlacklist2JSONString(), &normalized))
	assert.Equal(t, "enforce", normalized.Mode)
	assert.Equal(t, []string{"group-a", "group-b"}, normalized.EnabledGroups)
	assert.Equal(t, []string{"secret", "中文"}, normalized.ContentWords)
	assert.Equal(t, []string{"xn--bcher-kva.example"}, normalized.Domains)
	assert.Equal(t, "请求内容不符合策略", normalized.BlockMessage)
	assert.Equal(t, 403, normalized.BlockStatusCode)
	require.NoError(t, CheckRequestBlacklist(RequestBlacklist2JSONString()))
}

func TestRequestBlacklistStatusCodeNormalization(t *testing.T) {
	preserveRequestBlacklist(t)
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "missing", raw: `{}`, want: 403},
		{name: "zero", raw: `{"block_status_code":0}`, want: 403},
		{name: "bad request", raw: `{"block_status_code":400}`, want: 400},
		{name: "forbidden", raw: `{"block_status_code":403}`, want: 403},
		{name: "service unavailable", raw: `{"block_status_code":503}`, want: 503},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, UpdateRequestBlacklistByJSONString(test.raw))
			assert.Equal(t, test.want, GetRequestBlacklistSnapshot().BlockStatusCode())
		})
	}
}

func TestRequestBlacklistEmptyConfigurationIsOff(t *testing.T) {
	preserveRequestBlacklist(t)
	require.NoError(t, UpdateRequestBlacklistByJSONString(""))
	snapshot := GetRequestBlacklistSnapshot()
	assert.Equal(t, RequestBlacklistModeOff, snapshot.Mode())
	assert.False(t, snapshot.ShouldInspectGroup("default"))
	assert.Equal(t, 403, snapshot.BlockStatusCode())
	assert.NotEmpty(t, snapshot.BlockMessage())
	assert.True(t, utf8.ValidString(snapshot.BlockMessage()))
}

func TestRequestBlacklistSnapshotConcurrentReplacement(t *testing.T) {
	preserveRequestBlacklist(t)
	configA := RequestBlacklistConfig{
		Mode:            "observe",
		ContentWords:    []string{"alpha"},
		BlockMessage:    "message-a",
		BlockStatusCode: 400,
	}
	configB := RequestBlacklistConfig{
		Mode:            "enforce",
		ContentWords:    []string{"beta"},
		BlockMessage:    "message-b",
		BlockStatusCode: 503,
	}
	require.NoError(t, UpdateRequestBlacklistByJSONString(marshalRequestBlacklistConfig(t, configA)))
	configBJSON := marshalRequestBlacklistConfig(t, configB)

	start := make(chan struct{})
	errors := make(chan error, 17)
	var wait sync.WaitGroup
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			snapshot := GetRequestBlacklistSnapshot()
			isA := snapshot.Mode() == "observe" && snapshot.BlockMessage() == "message-a" && snapshot.BlockStatusCode() == 400
			isB := snapshot.Mode() == "enforce" && snapshot.BlockMessage() == "message-b" && snapshot.BlockStatusCode() == 503
			if !isA && !isB {
				errors <- fmt.Errorf("read mixed snapshot: mode=%q message=%q status=%d", snapshot.Mode(), snapshot.BlockMessage(), snapshot.BlockStatusCode())
			}
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		if err := UpdateRequestBlacklistByJSONString(configBJSON); err != nil {
			errors <- err
		}
	}()
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
