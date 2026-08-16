package limiter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitScriptRestoresBucketExpiry(t *testing.T) {
	assert.Contains(t, rateLimitScript, "redis.call('EXPIRE'")
	assert.NotContains(t, strings.ReplaceAll(rateLimitScript, " ", ""), "--redis.call('EXPIRE'")
}
