package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachUpstreamRequestIdSourceCreatesAdminInfo(t *testing.T) {
	other := map[string]interface{}{}

	attachUpstreamRequestIdSource(other, "X-Oneapi-Request-Id")

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "X-Oneapi-Request-Id", adminInfo["upstream_request_id_source"])
}

func TestAttachUpstreamRequestIdSourcePreservesExistingAdminInfo(t *testing.T) {
	adminInfo := map[string]interface{}{
		"existing":                   "keep",
		"upstream_request_id_source": "old-source",
	}
	other := map[string]interface{}{"admin_info": adminInfo}

	attachUpstreamRequestIdSource(other, "X-Request-Id")

	storedAdminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "keep", storedAdminInfo["existing"])
	assert.Equal(t, "X-Request-Id", storedAdminInfo["upstream_request_id_source"])
}

func TestAttachUpstreamRequestIdSourceNoopsForNilOther(t *testing.T) {
	assert.NotPanics(t, func() { attachUpstreamRequestIdSource(nil, "X-Request-Id") })
}
