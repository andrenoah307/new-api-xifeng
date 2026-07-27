package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var (
	relayAdmissionActiveRequests  atomic.Int64
	relayAdmissionActiveBodyBytes atomic.Int64
)

// RelayAdmission rejects new relay work when a process-wide capacity boundary is
// reached. It never revokes a request after admission, including an established
// streaming response.
func RelayAdmission() gin.HandlerFunc {
	return newRelayAdmissionHandler(common.InitCgroupMemorySampler, common.IsCgroupMemoryPressure)
}

func newRelayAdmissionHandler(initMemorySampler func(), isMemoryPressure func() bool) gin.HandlerFunc {
	maxRequests := int64(common.RelayMaxConcurrentRequests)
	maxBodyBytes := common.RelayMaxActiveBodyBytes
	memoryBreakerEnabled := common.RelayMemoryBreakerHighPercent > 0
	if maxRequests <= 0 && maxBodyBytes <= 0 && !memoryBreakerEnabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	if memoryBreakerEnabled {
		initMemorySampler()
	}
	retryAfterSeconds := common.RelayAdmissionRetryAfterSeconds

	return func(c *gin.Context) {
		requestReserved := false
		if maxRequests > 0 {
			if !reserveRelayAdmissionCapacity(&relayAdmissionActiveRequests, 1, maxRequests) {
				rejectRelayAdmission(c, retryAfterSeconds, "too_many_concurrent_requests", "relay is handling too many concurrent requests")
				return
			}
			requestReserved = true
		}

		bodyBytesReserved := int64(0)
		contentLength := c.Request.ContentLength
		// ContentLength is the compressed size for gzip requests and therefore
		// underestimates their decompressed live set. Incremental reservation after
		// decompression is intentionally left as a follow-up observability item.
		if maxBodyBytes > 0 && contentLength > 0 {
			if !reserveRelayAdmissionCapacity(&relayAdmissionActiveBodyBytes, contentLength, maxBodyBytes) {
				if requestReserved {
					relayAdmissionActiveRequests.Add(-1)
				}
				rejectRelayAdmission(c, retryAfterSeconds, "request_body_budget_exhausted", "relay request body budget is exhausted")
				return
			}
			bodyBytesReserved = contentLength
		}

		if memoryBreakerEnabled && isMemoryPressure() {
			if bodyBytesReserved > 0 {
				relayAdmissionActiveBodyBytes.Add(-bodyBytesReserved)
			}
			if requestReserved {
				relayAdmissionActiveRequests.Add(-1)
			}
			rejectRelayAdmission(c, retryAfterSeconds, "memory_pressure", "relay is unavailable because of cgroup memory pressure")
			return
		}

		// c.Next returns only after the handler lifecycle completes, including a
		// streaming response. These reservations are never used to cancel admitted
		// work; they are released only when that lifecycle has fully returned.
		defer func() {
			if bodyBytesReserved > 0 {
				relayAdmissionActiveBodyBytes.Add(-bodyBytesReserved)
			}
			if requestReserved {
				relayAdmissionActiveRequests.Add(-1)
			}
		}()
		c.Next()
	}
}

// A positive limit is an intentional hard upper bound: amount > limit is rejected
// even when the process is otherwise idle. RELAY_MAX_ACTIVE_BODY_BYTES must be set
// significantly above the largest valid single request body, or that request can
// never be admitted and retrying its 503 response cannot make it serviceable.
func reserveRelayAdmissionCapacity(counter *atomic.Int64, amount int64, limit int64) bool {
	if amount <= 0 || limit <= 0 || amount > limit {
		return false
	}
	for {
		current := counter.Load()
		if current < 0 || current > limit-amount {
			return false
		}
		if counter.CompareAndSwap(current, current+amount) {
			return true
		}
	}
}

func rejectRelayAdmission(c *gin.Context, retryAfterSeconds int, errorCode types.ErrorCode, message string) {
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	err := types.NewErrorWithStatusCode(errors.New(message), errorCode, http.StatusServiceUnavailable)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
		c.JSON(err.StatusCode, gin.H{"error": err.ToClaudeError()})
	} else {
		c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
	}
	c.Abort()
}
