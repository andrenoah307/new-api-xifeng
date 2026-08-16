package limiter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitPeekScriptIsReadOnly(t *testing.T) {
	require.NotEmpty(t, rateLimitPeekScript)
	for _, forbidden := range []string{"HMSET", "ZADD", "EXPIRE", "PEXPIRE", "ZREMRANGEBYSCORE"} {
		assert.NotContains(t, strings.ToUpper(rateLimitPeekScript), forbidden)
	}
	assert.Contains(t, rateLimitPeekScript, "HMGET")
	assert.Contains(t, rateLimitPeekScript, "TIME")
}

func TestRateLimitScriptRestoresBucketExpiry(t *testing.T) {
	assert.Contains(t, rateLimitScript, "redis.call('EXPIRE'")
	assert.NotContains(t, rateLimitScript, "--redis.call('EXPIRE'")
}

func TestParsePeekResultDistinguishesMissingAndPresentBuckets(t *testing.T) {
	tests := []struct {
		name   string
		input  []interface{}
		tokens int64
		exists bool
		err    bool
	}{
		{name: "missing", input: []interface{}{"missing"}},
		{name: "present", input: []interface{}{"present", int64(7)}, tokens: 7, exists: true},
		{name: "textual integer", input: []interface{}{"present", "7"}, tokens: 7, exists: true},
		{name: "fractional", input: []interface{}{"present", "7.5"}, err: true},
		{name: "malformed", input: []interface{}{"unknown"}, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, exists, err := parsePeekResult(tt.input)
			assert.Equal(t, tt.tokens, tokens)
			assert.Equal(t, tt.exists, exists)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPeekRejectsUninitializedLimiter(t *testing.T) {
	_, _, err := (*RedisLimiter)(nil).Peek(nil, "bucket")
	assert.Error(t, err)
}
