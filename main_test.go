package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPServerTimeoutPolicy(t *testing.T) {
	oldReadHeader := common.ServerReadHeaderTimeout
	oldIdle := common.ServerIdleTimeout
	t.Cleanup(func() {
		common.ServerReadHeaderTimeout = oldReadHeader
		common.ServerIdleTimeout = oldIdle
	})
	common.ServerReadHeaderTimeout = 20
	common.ServerIdleTimeout = 120

	srv := newHTTPServer(":0", http.NewServeMux())
	require.NotNil(t, srv)
	assert.Equal(t, 20*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 120*time.Second, srv.IdleTimeout)
	assert.Zero(t, srv.ReadTimeout)
	assert.Zero(t, srv.WriteTimeout)
}

func TestNewHTTPServerTimeoutsCanBeDisabled(t *testing.T) {
	oldReadHeader := common.ServerReadHeaderTimeout
	oldIdle := common.ServerIdleTimeout
	t.Cleanup(func() {
		common.ServerReadHeaderTimeout = oldReadHeader
		common.ServerIdleTimeout = oldIdle
	})
	common.ServerReadHeaderTimeout = 0
	common.ServerIdleTimeout = 0

	srv := newHTTPServer(":0", http.NewServeMux())
	assert.Zero(t, srv.ReadHeaderTimeout)
	assert.Zero(t, srv.IdleTimeout)
	assert.Zero(t, srv.ReadTimeout)
	assert.Zero(t, srv.WriteTimeout)
}
