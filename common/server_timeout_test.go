package common

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitServerTimeoutEnvDefaultsAndZeroDisables(t *testing.T) {
	oldReadHeader := ServerReadHeaderTimeout
	oldIdle := ServerIdleTimeout
	originalEnv := make(map[string]string)
	wasSet := make(map[string]bool)
	for _, name := range []string{"SERVER_READ_HEADER_TIMEOUT", "SERVER_IDLE_TIMEOUT"} {
		originalEnv[name], wasSet[name] = os.LookupEnv(name)
	}
	t.Cleanup(func() {
		ServerReadHeaderTimeout = oldReadHeader
		ServerIdleTimeout = oldIdle
		for name, value := range originalEnv {
			if wasSet[name] {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		}
	})

	for _, name := range []string{"SERVER_READ_HEADER_TIMEOUT", "SERVER_IDLE_TIMEOUT"} {
		require.NoError(t, os.Unsetenv(name))
	}
	initServerTimeoutEnv()
	assert.Equal(t, 20, ServerReadHeaderTimeout)
	assert.Equal(t, 120, ServerIdleTimeout)

	t.Setenv("SERVER_READ_HEADER_TIMEOUT", "0")
	t.Setenv("SERVER_IDLE_TIMEOUT", "0")
	initServerTimeoutEnv()
	assert.Zero(t, ServerReadHeaderTimeout)
	assert.Zero(t, ServerIdleTimeout)
}
