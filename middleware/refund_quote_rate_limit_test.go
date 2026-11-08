package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundQuoteCooldownExactInterval(t *testing.T) {
	gate := refundQuoteCooldown{next: make(map[int]time.Time)}
	start := time.Unix(100, 900000000)
	require.True(t, gate.allow(1, start))
	assert.False(t, gate.allow(1, start.Add(100*time.Millisecond)), "crossing a wall-clock second cannot bypass the interval")
	assert.False(t, gate.allow(1, start.Add(999*time.Millisecond)))
	assert.True(t, gate.allow(2, start), "independent viewers do not block each other")
	assert.True(t, gate.allow(1, start.Add(time.Second)))
}

func TestRefundQuoteRateLimitRedisAtomicInterval(t *testing.T) {
	server := setupRateLimitCapacityRedis(t)
	newEngine := func(actor int) *gin.Engine {
		engine := gin.New()
		engine.Use(func(c *gin.Context) { c.Set("id", actor) })
		engine.GET("/quote", RefundQuoteRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
		return engine
	}
	engines := []*gin.Engine{newEngine(9), newEngine(9)}
	results := make(chan int, 2)
	var workers sync.WaitGroup
	for _, engine := range engines {
		workers.Add(1)
		go func(engine *gin.Engine) {
			defer workers.Done()
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/quote", nil))
			results <- response.Code
		}(engine)
	}
	workers.Wait()
	close(results)
	codes := []int{}
	for code := range results {
		codes = append(codes, code)
	}
	assert.ElementsMatch(t, []int{http.StatusOK, http.StatusServiceUnavailable}, codes)
	server.FastForward(999 * time.Millisecond)
	response := httptest.NewRecorder()
	engines[0].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/quote", nil))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Equal(t, "1", response.Header().Get("Retry-After"))
	server.FastForward(time.Millisecond)
	response = httptest.NewRecorder()
	engines[1].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/quote", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	server.Close()
	response = httptest.NewRecorder()
	engines[0].ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/quote", nil))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, "Redis failure must not bypass the hard interval")
}
