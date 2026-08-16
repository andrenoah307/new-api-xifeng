package user_model_rpm

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var redisCallPattern = regexp.MustCompile(`redis\.call\(\s*'([A-Za-z]+)'`)

// redisCommandsIn returns the distinct commands a script actually issues.
// Matching on the call site instead of scanning raw text keeps the assertion
// immune to identifiers that merely embed a command name (for example the
// local variable `model` contains "del").
func redisCommandsIn(t *testing.T, path string) []string {
	t.Helper()
	script, err := os.ReadFile(path)
	require.NoError(t, err)
	seen := make(map[string]struct{})
	for _, match := range redisCallPattern.FindAllStringSubmatch(string(script), -1) {
		seen[strings.ToUpper(match[1])] = struct{}{}
	}
	commands := make([]string, 0, len(seen))
	for command := range seen {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	return commands
}

func TestUserModelRPMInspectLuaTrimsButNeverWritesEvents(t *testing.T) {
	assert.Equal(t,
		[]string{"TIME", "ZCARD", "ZRANGEBYSCORE", "ZREMRANGEBYSCORE"},
		redisCommandsIn(t, "lua/inspect.lua"),
		"inspect may only trim; any other write would let a dashboard read mutate observations",
	)

	script, err := os.ReadFile("lua/inspect.lua")
	require.NoError(t, err)
	source := string(script)
	// The member layout is a cross-language contract: Lua splits on the same
	// byte that memberFor joins with.
	assert.Contains(t, source, "string.char(31)")
	assert.Equal(t, "\x1f", memberSeparator)
	// Trimming must not renew the key's TTL, so the lower bound is scanned
	// from -inf rather than from an assumed retention floor.
	assert.Contains(t, source, "'-inf'")
}

func TestUserModelRPMRecordLuaUsesUniqueMemberAndBoundedTTL(t *testing.T) {
	assert.Equal(t,
		[]string{"PEXPIRE", "TIME", "ZADD", "ZREMRANGEBYSCORE"},
		redisCommandsIn(t, "lua/record.lua"),
		"record may only trim, insert and re-arm the TTL",
	)

	script, err := os.ReadFile("lua/record.lua")
	require.NoError(t, err)
	source := string(script)
	// NX is what makes a retried request idempotent instead of double counted.
	assert.Contains(t, source, "'NX'")
	assert.Contains(t, source, "'-inf'")
}

func TestUserModelRPMLuaWindowMatchesGoConstants(t *testing.T) {
	for _, path := range []string{"lua/record.lua", "lua/inspect.lua"} {
		script, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(script), "60000", path)
	}
	record, err := os.ReadFile("lua/record.lua")
	require.NoError(t, err)
	assert.Contains(t, string(record), "65000")

	assert.EqualValues(t, 60000, windowMillis)
	assert.EqualValues(t, 65000, ttlMillis)
}
