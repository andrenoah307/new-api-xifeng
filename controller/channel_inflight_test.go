/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/inflight"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callChannelInflight(t *testing.T, handler gin.HandlerFunc, method string, id string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/channel/"+id+"/inflight", nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	handler(c)

	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &payload), recorder.Body.String())
	return recorder.Code, payload
}

func TestChannelInflightRejectsUnusableChannelIds(t *testing.T) {
	for _, id := range []string{"abc", "0", "-3", ""} {
		t.Run("id="+id, func(t *testing.T) {
			_, preview := callChannelInflight(t, GetChannelInflight, http.MethodGet, id)
			assert.Equal(t, false, preview["success"])
			_, cleanup := callChannelInflight(t, CleanupChannelInflight, http.MethodPost, id)
			assert.Equal(t, false, cleanup["success"])
		})
	}
}

func TestChannelInflightPreviewReportsLiveConnections(t *testing.T) {
	const channelId = 930001
	entry := inflight.Register(channelId, true, func() {}, nil)
	require.NotNil(t, entry)
	t.Cleanup(entry.Release)

	status, payload := callChannelInflight(t, GetChannelInflight, http.MethodGet, strconv.Itoa(channelId))
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, payload["success"])
	data := payload["data"].(map[string]any)
	assert.Equal(t, float64(channelId), data["channel_id"])
	assert.Equal(t, float64(1), data["in_flight"])
	assert.Equal(t, float64(1), data["awaiting_headers"])
	assert.Equal(t, float64(0), data["cancellable"])
	assert.Equal(t, true, data["preview_only"], "the preview endpoint must never tear anything down")
	// The console needs the configured contracts to explain why a class is uncleanable.
	assert.Contains(t, data, "thresholds")
	assert.Contains(t, data, "threshold_enabled")
	assert.Equal(t, "local_instance", data["scope"], "counts are per instance, the UI must say so")
}

func TestChannelInflightCleanupSparesHealthyConnections(t *testing.T) {
	const channelId = 930002
	entry := inflight.Register(channelId, true, func() {}, nil)
	require.NotNil(t, entry)
	t.Cleanup(entry.Release)

	status, payload := callChannelInflight(t, CleanupChannelInflight, http.MethodPost, strconv.Itoa(channelId))
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, payload["success"])
	data := payload["data"].(map[string]any)
	assert.Equal(t, float64(1), data["in_flight"])
	assert.Equal(t, float64(0), data["cancelled"], "a connection inside its timeout contract is not overdue")
	assert.Equal(t, false, data["preview_only"])

	// Still tracked, so the request itself was untouched.
	assert.Equal(t, 1, inflight.Preview(channelId).InFlight)
}

func TestChannelInflightAnswersForUnknownChannelsWithoutDatabase(t *testing.T) {
	// The console polls the runtime endpoint on its refresh beat; verifying
	// existence per poll would trade an observability panel for database load.
	status, payload := callChannelInflight(t, GetChannelInflight, http.MethodGet, "939999")
	require.Equal(t, http.StatusOK, status)
	data := payload["data"].(map[string]any)
	assert.Equal(t, float64(0), data["in_flight"])
	assert.Equal(t, float64(0), data["oldest_age_ms"])
}

// One request per refresh, not one per visible row: the table reads every channel
// from this single response.
func TestChannelInflightRuntimeReportsAllChannelsAtOnce(t *testing.T) {
	const channelId = 930003
	entry := inflight.Register(channelId, true, func() {}, nil)
	require.NotNil(t, entry)
	t.Cleanup(entry.Release)

	status, payload := callChannelInflight(t, GetChannelInflightRuntime, http.MethodGet, "runtime")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, payload["success"])
	data := payload["data"].(map[string]any)
	assert.Equal(t, "local_instance", data["scope"])
	assert.Contains(t, data, "thresholds")
	assert.Contains(t, data, "threshold_enabled")

	channels, ok := data["channels"].([]any)
	require.True(t, ok, "channels must always be a list, never null")
	var found map[string]any
	for _, channel := range channels {
		entry := channel.(map[string]any)
		if entry["channel_id"] == float64(channelId) {
			found = entry
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, float64(1), found["in_flight"])
	assert.Equal(t, float64(0), found["cancellable"])
}
