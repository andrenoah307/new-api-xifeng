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
	Personal           *personalFinanceStat       `json:"personal,omitempty"`
	Storage            string                     `json:"storage"`
	SummaryDays        int                        `json:"summary_days"`
	RawDays            int                        `json:"raw_days"`
	SummaryGeneratedAt int64                      `json:"summary_generated_at"`
}

type personalFinanceStat struct {
	ConsumeQuota string `json:"consume_quota"`
	RefundQuota  string `json:"refund_quota"`
	NetQuota     string `json:"net_quota"`
	ConsumeCount int64  `json:"consume_count"`
	RefundCount  int64  `json:"refund_count"`
}

func financeSnapshotResponse(snapshot channelFinanceSnapshot) any {
	if snapshot.Personal == nil {
		return snapshot
	}
	return gin.H{"start_timestamp": snapshot.Start, "end_timestamp": snapshot.End, "requested_end_timestamp": snapshot.RequestedEnd, "next_start_timestamp": snapshot.NextStart, "complete": snapshot.Complete, "generated_at": snapshot.GeneratedAt, "personal": snapshot.Personal, "storage": snapshot.Storage, "summary_days": snapshot.SummaryDays, "raw_days": snapshot.RawDays, "summary_generated_at": snapshot.SummaryGeneratedAt}
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
	getFinanceStats(c, false)
}

func GetPersonalFinanceStats(c *gin.Context) {
	getFinanceStats(c, true)
}

func getFinanceStats(c *gin.Context, personal bool) {
	c.Header("Cache-Control", "no-store")
	minimumRole := common.RoleAdminUser
	if personal {
		minimumRole = common.RoleCommonUser
	}
	if c.GetInt("id") <= 0 || c.GetInt("role") < minimumRole {
		logRootUnavailable(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}
	var user model.User
	if err := model.DB.WithContext(c.Request.Context()).Select("id, role, status").First(&user, c.GetInt("id")).Error; err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	if user.Role < minimumRole || user.Status != common.UserStatusEnabled {
		logRootUnavailable(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}
	start, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.DefaultQuery("end_timestamp", strconv.FormatInt((time.Now().Unix()-120)/60*60, 10)), 10, 64)
	channel, channelErr := strconv.Atoi(c.DefaultQuery("channel", "0"))
	window, windowErr := strconv.ParseInt(c.DefaultQuery("window_seconds", "86400"), 10, 64)
	if err != nil || endErr != nil || channelErr != nil || windowErr != nil || window < 1 || window > 86400 || start < 1 || end <= start || end > time.Now().Unix() || channel < 0 {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	requestedEnd := end
	granularity := c.DefaultQuery("granularity", "day")
	if granularity != "day" && granularity != "month" || personal && channel != 0 {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	if granularity == "month" {
		t := time.Unix(start, 0).UTC()
		monthEnd := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC).Unix()
		if end > monthEnd {
			end = monthEnd
		}
	} else if end-start > window {
		end = start + window
	}
	userID := 0
	if personal {
		userID = user.Id
	}
	key := fmt.Sprintf("%p:%s:%d:%d:%d:%d:%d", model.LOG_DB, common.LogDatabaseType(), start, end, channel, requestedEnd, userID)
	channelFinanceCache.Lock()
	cached := channelFinanceCache.snapshot
	hit := !common.UsingLogDatabase(common.DatabaseTypeClickHouse) && channelFinanceCache.key == key && time.Now().Before(channelFinanceCache.expires)
	channelFinanceCache.Unlock()
	if hit {
		common.ApiSuccess(c, financeSnapshotResponse(cached))
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
	windowResult, err := model.GetFinanceWindow(ctx, start, end, channel, userID)
	if err != nil {
		// Database errors may contain raw metadata or SQL: never return them here.
		if ctx.Err() != nil {
			handleLogQueryError(c, ctx.Err())
		} else {
			logRootUnavailable(c, i18n.MsgDatabaseError)
		}
		return
	}
	end = windowResult.End
	rows := windowResult.Channels
	snapshot := channelFinanceSnapshot{Start: start, End: end, RequestedEnd: requestedEnd, Complete: end == requestedEnd, GeneratedAt: time.Now().Unix(), Channels: rows, RefundCostResolved: true, Storage: windowResult.Storage, SummaryDays: windowResult.SummaryDays, RawDays: windowResult.RawDays, SummaryGeneratedAt: windowResult.SummaryGeneratedAt}
	if !snapshot.Complete {
		snapshot.NextStart = end
	}
	for _, row := range rows {
		if row.UnresolvedRefunds > 0 {
			snapshot.RefundCostResolved = false
			break
		}
	}
	if personal {
		snapshot.Channels = nil
		snapshot.Personal = &personalFinanceStat{ConsumeQuota: "0", RefundQuota: "0", NetQuota: "0"}
		if len(rows) > 0 {
			row := rows[0]
			snapshot.Personal = &personalFinanceStat{ConsumeQuota: row.ConsumeQuota, RefundQuota: row.RefundQuota, NetQuota: row.NetQuota, ConsumeCount: row.ConsumeCount, RefundCount: row.RefundCount}
		}
	}
	channelFinanceCache.Lock()
	channelFinanceCache.key = key
	channelFinanceCache.snapshot = snapshot
	channelFinanceCache.expires = time.Now().Add(time.Minute)
	channelFinanceCache.Unlock()
	common.ApiSuccess(c, financeSnapshotResponse(snapshot))
}
