package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func rootLogPageSize() int {
	size := common.GetEnvOrDefault("ROOT_LOG_PAGE_SIZE", 10000)
	if size < 100 {
		return 100
	}
	if size > 10000 {
		return 10000
	}
	return size
}

func logRootUnavailable(c *gin.Context, key string) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": common.TranslateMessage(c, key)})
}

func authorizeLogRoot(c *gin.Context) bool {
	if c.GetInt("role") != common.RoleRootUser || c.GetInt("id") <= 0 {
		logRootUnavailable(c, i18n.MsgAuthInsufficientPrivilege)
		return false
	}
	var user model.User
	if err := model.DB.WithContext(c.Request.Context()).Select("id, role, status").First(&user, c.GetInt("id")).Error; err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return false
	}
	if user.Role != common.RoleRootUser || user.Status != common.UserStatusEnabled {
		logRootUnavailable(c, i18n.MsgAuthInsufficientPrivilege)
		return false
	}
	return true
}

type channelFinanceSnapshot struct {
	Start              int64                      `json:"start_timestamp"`
	End                int64                      `json:"end_timestamp"`
	RequestedEnd       int64                      `json:"requested_end_timestamp"`
	NextStart          int64                      `json:"next_start_timestamp"`
	Complete           bool                       `json:"complete"`
	GeneratedAt        int64                      `json:"generated_at"`
	Channels           []model.ChannelFinanceStat `json:"channels"`
	RefundCostResolved bool                       `json:"refund_cost_resolved"`
}

var channelFinanceCache = struct {
	sync.Mutex
	key      string
	snapshot channelFinanceSnapshot
	expires  time.Time
}{}
var channelFinanceQuery sync.Mutex

// GetChannelFinanceStats returns bounded aggregate output, not paged ledger rows.
func GetChannelFinanceStats(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !authorizeLogRoot(c) {
		return
	}
	start, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.DefaultQuery("end_timestamp", strconv.FormatInt((time.Now().Unix()-120)/60*60, 10)), 10, 64)
	channel, channelErr := strconv.Atoi(c.DefaultQuery("channel", "0"))
	window, windowErr := strconv.ParseInt(c.DefaultQuery("window_seconds", "3600"), 10, 64)
	if err != nil || endErr != nil || channelErr != nil || windowErr != nil || window < 1 || window > 86400 || start < 1 || end <= start || end > time.Now().Unix() || channel < 0 {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	requestedEnd := end
	if end-start > window {
		end = start + window
	}
	key := fmt.Sprintf("%p:%s:%d:%d:%d:%d", model.LOG_DB, common.LogDatabaseType(), start, end, channel, requestedEnd)
	channelFinanceCache.Lock()
	cached := channelFinanceCache.snapshot
	hit := channelFinanceCache.key == key && time.Now().Before(channelFinanceCache.expires)
	channelFinanceCache.Unlock()
	if hit {
		common.ApiSuccess(c, cached)
		return
	}
	if !channelFinanceQuery.TryLock() {
		c.Header("Retry-After", "3")
		logRootUnavailable(c, i18n.MsgLogQueryTimeout)
		return
	}
	defer channelFinanceQuery.Unlock()
	seconds := common.GetEnvOrDefault("ROOT_LOG_STAT_TIMEOUT", 180)
	if seconds < 1 {
		seconds = 180
	}
	if seconds > 600 {
		seconds = 600
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(seconds)*time.Second)
	defer cancel()
	rows, err := model.GetChannelFinanceStats(ctx, start, end, channel)
	if err != nil {
		// Database errors may contain raw metadata or SQL: never return them here.
		if ctx.Err() != nil {
			handleLogQueryError(c, ctx.Err())
		} else {
			logRootUnavailable(c, i18n.MsgDatabaseError)
		}
		return
	}
	snapshot := channelFinanceSnapshot{Start: start, End: end, RequestedEnd: requestedEnd, Complete: end == requestedEnd, GeneratedAt: time.Now().Unix(), Channels: rows, RefundCostResolved: true}
	if !snapshot.Complete {
		snapshot.NextStart = end
	}
	for _, row := range rows {
		if row.UnresolvedRefunds > 0 {
			snapshot.RefundCostResolved = false
			break
		}
	}
	channelFinanceCache.Lock()
	channelFinanceCache.key = key
	channelFinanceCache.snapshot = snapshot
	channelFinanceCache.expires = time.Now().Add(time.Minute)
	channelFinanceCache.Unlock()
	common.ApiSuccess(c, snapshot)
}
