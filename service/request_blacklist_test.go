package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequestBlacklistForTest(t *testing.T, config setting.RequestBlacklistConfig) {
	t.Helper()
	previous := setting.RequestBlacklist2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateRequestBlacklistByJSONString(previous))
	})
	data, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateRequestBlacklistByJSONString(string(data)))
}

func TestInspectRequestBlacklistDomainBoundaries(t *testing.T) {
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:    "enforce",
		Domains: []string{"example.com"},
	})
	tests := []struct {
		name string
		text string
		hit  bool
	}{
		{name: "bare domain", text: "example.com", hit: true},
		{name: "url", text: "https://example.com/x", hit: true},
		{name: "subdomain", text: "a.example.com", hit: true},
		{name: "embedded prefix", text: "notexample.com", hit: false},
		{name: "parent of another domain", text: "example.com.evil.com", hit: false},
		{name: "longer suffix", text: "example.community", hit: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := InspectRequestBlacklist("default", test.text)
			assert.Equal(t, test.hit, decision.Hit)
			if test.hit {
				assert.Equal(t, "domain", decision.MatchKind)
			}
		})
	}
}

func TestInspectRequestBlacklistMatchesIDNAsPunycode(t *testing.T) {
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:    "enforce",
		Domains: []string{"例え.jp"},
	})
	decision := InspectRequestBlacklist("default", "target=xn--r8jz45g.jp")
	assert.True(t, decision.Hit)
	assert.Equal(t, "domain", decision.MatchKind)

	decision = InspectRequestBlacklist("default", "target=例え.jp")
	assert.False(t, decision.Hit, "raw Unicode domains are not IDNA-normalized on the scan path")
}

func TestInspectRequestBlacklistContentSemantics(t *testing.T) {
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:         "enforce",
		ContentWords: []string{"Needle", "敏感内容", "🔒"},
	})
	tests := []struct {
		name string
		text string
		hit  bool
	}{
		{name: "substring", text: "prefixneedlesuffix", hit: true},
		{name: "case insensitive", text: "NEEDLE", hit: true},
		{name: "chinese", text: "这里有敏感内容", hit: true},
		{name: "emoji", text: "locked 🔒 value", hit: true},
		{name: "miss", text: "ordinary text", hit: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := InspectRequestBlacklist("default", test.text)
			assert.Equal(t, test.hit, decision.Hit)
			if test.hit {
				assert.Equal(t, "content", decision.MatchKind)
			}
		})
	}
}

func TestInspectRequestBlacklistContentUsesRuneLowercaseMapping(t *testing.T) {
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:         "enforce",
		ContentWords: []string{"İ"},
	})

	decision := InspectRequestBlacklist("default", "i")
	assert.True(t, decision.Hit)
	assert.Equal(t, "content", decision.MatchKind)
}

func TestInspectRequestBlacklistSlidingWindows(t *testing.T) {
	const step = 32 * 1024
	tests := []struct {
		name string
		word string
		text string
	}{
		{
			name: "crosses boundary",
			word: "cross-boundary",
			text: strings.Repeat("x", step-5) + "cross-boundary" + strings.Repeat("x", step+31),
		},
		{
			name: "starts at window boundary",
			word: "at-boundary",
			text: strings.Repeat("x", step) + "at-boundary" + strings.Repeat("x", step+17),
		},
		{
			name: "ends with text",
			word: "at-the-end",
			text: strings.Repeat("x", 2*step+19) + "at-the-end",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{Mode: "enforce", ContentWords: []string{test.word}})
			decision, accepted := inspectRequestBlacklistText(setting.GetRequestBlacklistSnapshot(), test.text, false)
			assert.True(t, decision.Hit)
			assert.Equal(t, "content", decision.MatchKind)
			assert.Equal(t, 1, accepted, "one physical match must be adopted by exactly one window")
		})
	}
}

func TestInspectRequestBlacklistTailWindowContentBoundaries(t *testing.T) {
	const word = "needle"
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:         "enforce",
		ContentWords: []string{word},
	})
	snapshot := setting.GetRequestBlacklistSnapshot()
	pad := snapshot.MaxPatternRunes() + 1
	offsets := []int{
		requestBlacklistWindowStep - 1,
		requestBlacklistWindowStep,
		requestBlacklistWindowStep + 1,
		requestBlacklistWindowStep + pad - 2,
		requestBlacklistWindowStep + pad - 1,
		requestBlacklistWindowStep + pad,
		2 * requestBlacklistWindowStep,
		2*requestBlacklistWindowStep + pad - 1,
	}

	for _, offset := range offsets {
		t.Run(fmt.Sprintf("offset_%d", offset), func(t *testing.T) {
			text := strings.Repeat("x", offset) + word
			decision, accepted := inspectRequestBlacklistText(snapshot, text, false)
			assert.True(t, decision.Hit)
			assert.Equal(t, "content", decision.MatchKind)
			assert.Equal(t, 1, accepted, "one physical match must be adopted by exactly one window")
		})
	}
}

func TestInspectRequestBlacklistTailWindowDomainBoundaries(t *testing.T) {
	const domain = "example.com"
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:    "enforce",
		Domains: []string{domain},
	})
	snapshot := setting.GetRequestBlacklistSnapshot()
	pad := snapshot.MaxPatternRunes() + 1
	offsets := []int{
		requestBlacklistWindowStep - 1,
		requestBlacklistWindowStep,
		requestBlacklistWindowStep + 1,
		requestBlacklistWindowStep + pad - 2,
		requestBlacklistWindowStep + pad - 1,
		requestBlacklistWindowStep + pad,
		2 * requestBlacklistWindowStep,
		2*requestBlacklistWindowStep + pad - 1,
	}

	for _, offset := range offsets {
		t.Run(fmt.Sprintf("offset_%d", offset), func(t *testing.T) {
			text := strings.Repeat("x", offset-1) + " " + domain
			decision, accepted := inspectRequestBlacklistText(snapshot, text, false)
			assert.True(t, decision.Hit)
			assert.Equal(t, "domain", decision.MatchKind)
			assert.Equal(t, 1, accepted, "one physical match must be adopted by exactly one window")
		})
	}

	t.Run("rejects invalid left neighbor in tail window", func(t *testing.T) {
		text := strings.Repeat("x", requestBlacklistWindowStep) + domain
		decision, accepted := inspectRequestBlacklistText(snapshot, text, false)
		assert.False(t, decision.Hit)
		assert.Zero(t, accepted)
	})
	t.Run("rejects invalid right neighbor in tail window", func(t *testing.T) {
		text := strings.Repeat("x", requestBlacklistWindowStep-1) + " " + domain + ".evil.com"
		decision, accepted := inspectRequestBlacklistText(snapshot, text, false)
		assert.False(t, decision.Hit)
		assert.Zero(t, accepted)
	})
}

func TestInspectRequestBlacklistModesAndGroups(t *testing.T) {
	tests := []struct {
		name        string
		config      setting.RequestBlacklistConfig
		group       string
		wantHit     bool
		wantEnforce bool
	}{
		{name: "off", config: setting.RequestBlacklistConfig{Mode: "off", ContentWords: []string{"blocked"}}, group: "default"},
		{name: "observe", config: setting.RequestBlacklistConfig{Mode: "observe", ContentWords: []string{"blocked"}}, group: "default", wantHit: true},
		{name: "enforce", config: setting.RequestBlacklistConfig{Mode: "enforce", ContentWords: []string{"blocked"}, BlockStatusCode: 503}, group: "default", wantHit: true, wantEnforce: true},
		{name: "group excluded", config: setting.RequestBlacklistConfig{Mode: "enforce", EnabledGroups: []string{"paid"}, ContentWords: []string{"blocked"}}, group: "default"},
		{name: "group included", config: setting.RequestBlacklistConfig{Mode: "enforce", EnabledGroups: []string{"paid"}, ContentWords: []string{"blocked"}}, group: "paid", wantHit: true, wantEnforce: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setRequestBlacklistForTest(t, test.config)
			decision := InspectRequestBlacklist(test.group, "blocked")
			assert.Equal(t, test.wantHit, decision.Hit)
			assert.Equal(t, test.wantEnforce, decision.IsEnforced())
		})
	}
}

func TestRequestBlacklistAuditLimiterIsBoundedAndReportsSuppression(t *testing.T) {
	limiter := newRequestBlacklistAuditLimiter(2, time.Minute)
	now := time.Unix(1_700_000_000, 0)

	logCurrent, suppressed := limiter.allow("token:1:content", now)
	assert.True(t, logCurrent)
	assert.Zero(t, suppressed)
	logCurrent, suppressed = limiter.allow("token:1:content", now.Add(time.Second))
	assert.False(t, logCurrent)
	assert.Zero(t, suppressed)
	logCurrent, _ = limiter.allow("token:2:content", now.Add(2*time.Second))
	assert.True(t, logCurrent)
	logCurrent, suppressed = limiter.allow("token:3:domain", now.Add(time.Minute))
	assert.True(t, logCurrent)
	assert.Equal(t, uint64(1), suppressed)
	assert.LessOrEqual(t, limiter.size(), 2)
}

func TestAuditRequestBlacklistHitIsStructuredRedactedAndRateLimited(t *testing.T) {
	const rule = "do-not-log-this-rule-84721"
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:         "observe",
		ContentWords: []string{rule},
	})
	decision := InspectRequestBlacklist("audit-group", "prefix "+rule+" suffix")
	require.True(t, decision.Hit)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "audit-request-84721")
	common.SetContextKey(c, constant.ContextKeyUserId, 184721)
	common.SetContextKey(c, constant.ContextKeyTokenId, 284721)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "audit-group")

	var output bytes.Buffer
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &output
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousWriter
		common.LogWriterMu.Unlock()
	})

	AuditRequestBlacklistHit(c, decision)
	firstLog := output.String()
	assert.Contains(t, firstLog, `request_id="audit-request-84721"`)
	assert.Contains(t, firstLog, "user_id=184721")
	assert.Contains(t, firstLog, "token_id=284721")
	assert.Contains(t, firstLog, `group="audit-group"`)
	assert.Contains(t, firstLog, `route="/v1/chat/completions"`)
	assert.Contains(t, firstLog, `mode="observe"`)
	assert.Contains(t, firstLog, `match_kind="content"`)
	assert.Contains(t, firstLog, `config_hash="`+decision.ConfigHash()+`"`)
	assert.NotContains(t, firstLog, rule)

	AuditRequestBlacklistHit(c, decision)
	assert.Equal(t, firstLog, output.String())
}

func TestBlacklistDecisionCarriesOneConsistentSnapshot(t *testing.T) {
	setRequestBlacklistForTest(t, setting.RequestBlacklistConfig{
		Mode:            "enforce",
		ContentWords:    []string{"blocked"},
		BlockMessage:    "policy message",
		BlockStatusCode: 400,
	})
	decision := InspectRequestBlacklist("default", "blocked")
	require.True(t, decision.Hit)
	assert.Equal(t, "enforce", decision.Mode())
	assert.Equal(t, "policy message", decision.BlockMessage())
	assert.Equal(t, 400, decision.BlockStatusCode())
	assert.NotEmpty(t, decision.ConfigHash())
	assert.NotContains(t, fmt.Sprint(decision.ConfigHash()), "blocked")
}
