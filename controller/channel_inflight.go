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
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relay/inflight"

	"github.com/gin-gonic/gin"
)

// All three handlers answer purely from the in-process registry. They deliberately
// do not verify that the channel exists: the admin console polls them on its
// refresh beat, and a database round trip per poll would turn an observability
// panel into load. An unknown id simply reports an empty channel.

// GetChannelInflight reports what one channel currently has in flight on this
// instance, without touching a single connection. It backs the confirmation
// dialog, which needs a reading fresher than the table's polling beat.
func GetChannelInflight(c *gin.Context) {
	channelId, ok := channelIdParam(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    inflight.Preview(channelId),
	})
}

// GetChannelInflightRuntime reports every channel that has something in flight on
// this instance. The channel table polls this one endpoint on its existing refresh
// beat; a per-row endpoint would multiply the poll by the number of visible rows.
func GetChannelInflightRuntime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    inflight.CurrentRuntime(),
	})
}

// CleanupChannelInflight tears down the connections of one channel that have
// already outlived the gateway's own timeout contract. Connections still inside
// their contract are never touched, so a healthy channel reports zero cleared.
func CleanupChannelInflight(c *gin.Context) {
	channelId, ok := channelIdParam(c)
	if !ok {
		return
	}
	snapshot := inflight.Cleanup(channelId)
	if snapshot.Cancelled > 0 {
		recordManageAudit(c, "channel.inflight_cleanup", map[string]interface{}{
			"id":        channelId,
			"count":     snapshot.Cancelled,
			"in_flight": snapshot.InFlight,
			"by_reason": snapshot.ByReason,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    snapshot,
	})
}

func channelIdParam(c *gin.Context) (int, bool) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return 0, false
	}
	return channelId, true
}
