package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

type refundQuoteCooldown struct {
	mu        sync.Mutex
	next      map[int]time.Time
	cleanupAt time.Time
}

func (limit *refundQuoteCooldown) allow(actor int, now time.Time) bool {
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if !now.Before(limit.cleanupAt) {
		for id, until := range limit.next {
			if !now.Before(until) {
				delete(limit.next, id)
			}
		}
		limit.cleanupAt = now.Add(time.Minute)
	}
	if now.Before(limit.next[actor]) {
		return false
	}
	limit.next[actor] = now.Add(time.Second)
	return true
}

// RefundQuoteRateLimit enforces a full second between a viewer's quote requests,
// independent of global rate-limit switches and administrator exemptions.
func RefundQuoteRateLimit() gin.HandlerFunc {
	memory := &refundQuoteCooldown{next: make(map[int]time.Time)}
	return func(c *gin.Context) {
		actor := c.GetInt("id")
		if actor <= 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		allowed := false
		if common.RedisEnabled {
			if common.RDB != nil {
				var err error
				allowed, err = common.RDB.SetNX(c.Request.Context(), "refund:quote:interval:"+strconv.Itoa(actor), "1", time.Second).Result()
				if err != nil {
					common.SysError("refund quote interval unavailable: " + err.Error())
				}
			}
		} else {
			allowed = memory.allow(actor, time.Now())
		}
		if !allowed {
			c.Header("Retry-After", "1")
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgRateLimitCapacityThrottled)})
		}
	}
}
