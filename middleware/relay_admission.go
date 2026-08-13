package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var (
	relayAdmissionActiveRequests         atomic.Int64
	relayAdmissionActiveBodyBytes        atomic.Int64
	relayAdmissionRejectedConcurrent     atomic.Uint64
	relayAdmissionRejectedBody           atomic.Uint64
	relayAdmissionRejectedMemory         atomic.Uint64
	relayAdmissionExemptedMemoryPressure atomic.Uint64
)

// RelayAdmissionStats is the process-wide relay admission state.
type RelayAdmissionStats struct {
	ActiveRequests                     int64  `json:"active_requests"`
	ActiveBodyBytes                    int64  `json:"active_body_bytes"`
	MaxConcurrentRequests              int    `json:"max_concurrent_requests"`
	MaxActiveBodyBytes                 int64  `json:"max_active_body_bytes"`
	RejectedTooManyConcurrentRequests  uint64 `json:"rejected_too_many_concurrent_requests"`
	RejectedRequestBodyBudgetExhausted uint64 `json:"rejected_request_body_budget_exhausted"`
	RejectedMemoryPressure             uint64 `json:"rejected_memory_pressure"`
	ExemptedMemoryPressure             uint64 `json:"exempted_memory_pressure"`
}

// GetRelayAdmissionStats returns a lock-free snapshot of relay admission state.
func GetRelayAdmissionStats() RelayAdmissionStats {
	return RelayAdmissionStats{
		ActiveRequests:                     relayAdmissionActiveRequests.Load(),
		ActiveBodyBytes:                    relayAdmissionActiveBodyBytes.Load(),
		MaxConcurrentRequests:              common.RelayMaxConcurrentRequests,
		MaxActiveBodyBytes:                 common.RelayMaxActiveBodyBytes,
		RejectedTooManyConcurrentRequests:  relayAdmissionRejectedConcurrent.Load(),
		RejectedRequestBodyBudgetExhausted: relayAdmissionRejectedBody.Load(),
		RejectedMemoryPressure:             relayAdmissionRejectedMemory.Load(),
		ExemptedMemoryPressure:             relayAdmissionExemptedMemoryPressure.Load(),
	}
}

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
			if isMemoryPressureExempt(c.Request) {
				relayAdmissionExemptedMemoryPressure.Add(1)
			} else {
				if bodyBytesReserved > 0 {
					relayAdmissionActiveBodyBytes.Add(-bodyBytesReserved)
				}
				if requestReserved {
					relayAdmissionActiveRequests.Add(-1)
				}
				rejectRelayAdmission(c, retryAfterSeconds, "memory_pressure", "relay is unavailable because of cgroup memory pressure")
				return
			}
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

// 内存跳闸只拦新的重负载提交：GET 不携带 relay 请求体，且其中的任务轮询/取结果
// 路径对应的工作已经被接纳并计费，跳闸期间拒绝它们等同于毁掉已付费的请求。
// WebSocket 升级（/v1/realtime）是长连接重负载，必须继续受闸门约束。
func isMemoryPressureExempt(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return !strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
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
	switch errorCode {
	case "too_many_concurrent_requests":
		relayAdmissionRejectedConcurrent.Add(1)
		recordRelayRejection(c, realtimemetrics.RejectionConcurrency)
	case "request_body_budget_exhausted":
		relayAdmissionRejectedBody.Add(1)
		recordRelayRejection(c, realtimemetrics.RejectionBody)
	case "memory_pressure":
		relayAdmissionRejectedMemory.Add(1)
		recordRelayRejection(c, realtimemetrics.RejectionMemory)
	}
	c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	err := types.NewErrorWithStatusCode(errors.New(message), errorCode, http.StatusServiceUnavailable)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
		c.JSON(err.StatusCode, gin.H{"error": err.ToClaudeError()})
	} else {
		c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
	}
	c.Abort()
}
