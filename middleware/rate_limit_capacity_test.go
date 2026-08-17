package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rateLimitCapacityMemoryUserID atomic.Int64

type rateLimitCapacityRedisErrorHook struct {
	commandName string
}

func (h rateLimitCapacityRedisErrorHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == h.commandName {
		return ctx, errors.New("injected Redis command error")
	}
	return ctx, nil
}

func (rateLimitCapacityRedisErrorHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (rateLimitCapacityRedisErrorHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (rateLimitCapacityRedisErrorHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func setupRateLimitCapacityRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
		require.NoError(t, client.Close())
	})
	require.NoError(t, i18n.Init())
	return server
}

func newRateLimitCapacityTestEngine(userID int, path string, gate gin.HandlerFunc) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	handled := new(int)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", userID)
	})
	engine.GET(path, gate, func(c *gin.Context) {
		(*handled)++
		c.Status(http.StatusOK)
	})
	return engine, handled
}

func performRateLimitCapacityRequest(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func captureRateLimitCapacitySysErrors(t *testing.T) *bytes.Buffer {
	t.Helper()

	buffer := new(bytes.Buffer)
	common.LogWriterMu.Lock()
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = buffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousWriter
		common.LogWriterMu.Unlock()
	})
	return buffer
}

func assertRateLimitCapacityThrottleResponse(t *testing.T, recorder *httptest.ResponseRecorder, userID int) {
	t.Helper()

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, strconv.FormatInt(rateLimitCapacityWindowSeconds, 10), recorder.Header().Get("Retry-After"))
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response, 3)
	assert.Equal(t, false, response["success"])
	message, ok := response["message"].(string)
	require.True(t, ok)
	assert.Equal(t, i18n.Translate(i18n.LangEn, i18n.MsgRateLimitCapacityThrottled), message)
	assert.Nil(t, response["data"])
	assert.NotRegexp(t, `[0-9]`, message)
	for _, internalValue := range []string{
		"rateLimit:RLC:user:",
		"RLC",
		"uid",
		strconv.Itoa(userID),
	} {
		assert.NotContains(t, message, internalValue)
	}
}

func TestRateLimitCapacityGateRedisAllowsThirtyRequestsAndSetsTTL(t *testing.T) {
	server := setupRateLimitCapacityRedis(t)
	const userID = 4101
	engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

	for requestNumber := 1; requestNumber <= rateLimitCapacityMaxRequests; requestNumber++ {
		recorder := performRateLimitCapacityRequest(engine, "/capacity")
		assert.Equalf(t, http.StatusOK, recorder.Code, "request %d", requestNumber)
	}

	assert.Equal(t, rateLimitCapacityMaxRequests, *handled)
	key := "rateLimit:RLC:user:" + strconv.Itoa(userID)
	entries, err := server.List(key)
	require.NoError(t, err)
	assert.Len(t, entries, rateLimitCapacityMaxRequests)
	assert.Greater(t, server.TTL(key), time.Duration(0))
}

func TestRateLimitCapacityGateRedisRejectsThirtyFirstRequestWithRetryableResponse(t *testing.T) {
	setupRateLimitCapacityRedis(t)
	const userID = 4202
	engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

	for requestNumber := 1; requestNumber <= rateLimitCapacityMaxRequests; requestNumber++ {
		recorder := performRateLimitCapacityRequest(engine, "/capacity")
		require.Equalf(t, http.StatusOK, recorder.Code, "request %d", requestNumber)
	}
	rejected := performRateLimitCapacityRequest(engine, "/capacity")

	assertRateLimitCapacityThrottleResponse(t, rejected, userID)
	assert.Equal(t, rateLimitCapacityMaxRequests, *handled)
}

func TestRateLimitCapacityGateRedisLLenErrorFailsOpen(t *testing.T) {
	server := setupRateLimitCapacityRedis(t)
	logOutput := captureRateLimitCapacitySysErrors(t)
	const userID = 4303
	key := "rateLimit:RLC:user:" + strconv.Itoa(userID)
	require.NoError(t, server.Set(key, "wrong-type"))
	engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

	recorder := performRateLimitCapacityRequest(engine, "/capacity")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEqual(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, 1, *handled)
	assert.Contains(t, logOutput.String(), "LLen")
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[SYS]"))
}

func TestRateLimitCapacityGateInvalidRedisTimestampFailsOpen(t *testing.T) {
	server := setupRateLimitCapacityRedis(t)
	logOutput := captureRateLimitCapacitySysErrors(t)
	const userID = 4404
	key := "rateLimit:RLC:user:" + strconv.Itoa(userID)
	for entry := 0; entry < rateLimitCapacityMaxRequests; entry++ {
		_, err := server.Lpush(key, "not-a-time")
		require.NoError(t, err)
	}
	engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

	recorder := performRateLimitCapacityRequest(engine, "/capacity")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEqual(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, 1, *handled)
	assert.Contains(t, strings.ToLower(logOutput.String()), "time")
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[SYS]"))
}

func TestRateLimitCapacityGateRedisCommandErrorsFailOpen(t *testing.T) {
	testCases := []struct {
		name        string
		commandName string
		bucketState string
	}{
		{name: "LPush below capacity", commandName: "lpush", bucketState: "empty"},
		{name: "Expire below capacity", commandName: "expire", bucketState: "empty"},
		{name: "LIndex at capacity", commandName: "lindex", bucketState: "recent"},
		{name: "Expire at capacity", commandName: "expire", bucketState: "recent"},
		{name: "LPush after window", commandName: "lpush", bucketState: "expired"},
		{name: "LTrim after window", commandName: "ltrim", bucketState: "expired"},
		{name: "Expire after window", commandName: "expire", bucketState: "expired"},
	}
	for testIndex, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := setupRateLimitCapacityRedis(t)
			logOutput := captureRateLimitCapacitySysErrors(t)
			userID := 4500 + testIndex
			key := "rateLimit:RLC:user:" + strconv.Itoa(userID)
			if testCase.bucketState != "empty" {
				entryTime := time.Now()
				if testCase.bucketState == "expired" {
					entryTime = entryTime.Add(-time.Duration(rateLimitCapacityWindowSeconds+1) * time.Second)
				}
				for entry := 0; entry < rateLimitCapacityMaxRequests; entry++ {
					_, err := server.Lpush(key, entryTime.Format(timeFormat))
					require.NoError(t, err)
				}
			}
			common.RDB.AddHook(rateLimitCapacityRedisErrorHook{commandName: testCase.commandName})
			engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

			recorder := performRateLimitCapacityRequest(engine, "/capacity")

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.NotEqual(t, http.StatusInternalServerError, recorder.Code)
			assert.Equal(t, 1, *handled)
			assert.Equal(t, 1, strings.Count(logOutput.String(), "[SYS]"))
		})
	}
}

func TestRateLimitCapacityGateMissingRedisClientFailsOpen(t *testing.T) {
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
	})
	require.NoError(t, i18n.Init())
	logOutput := captureRateLimitCapacitySysErrors(t)
	engine, handled := newRateLimitCapacityTestEngine(4555, "/capacity", RateLimitCapacityGate())

	recorder := performRateLimitCapacityRequest(engine, "/capacity")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 1, *handled)
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[SYS]"))
}

func TestRateLimitCapacityGateZeroUserIDFailsOpenWithoutSharedBucket(t *testing.T) {
	server := setupRateLimitCapacityRedis(t)
	logOutput := captureRateLimitCapacitySysErrors(t)
	engine, handled := newRateLimitCapacityTestEngine(0, "/capacity", RateLimitCapacityGate())

	recorder := performRateLimitCapacityRequest(engine, "/capacity")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEqual(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, 1, *handled)
	assert.False(t, server.Exists("rateLimit:RLC:user:0"))
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[SYS]"))
}

func TestRateLimitCapacityGateKeepsCapacityAndDashboardBudgetsIndependent(t *testing.T) {
	setupRateLimitCapacityRedis(t)
	previousEnabled := common.DashboardDataRateLimitEnable
	previousLimit := common.DashboardDataRateLimitNum
	previousDuration := common.DashboardDataRateLimitDuration
	common.DashboardDataRateLimitEnable = true
	common.DashboardDataRateLimitNum = 1
	common.DashboardDataRateLimitDuration = rateLimitCapacityWindowSeconds
	t.Cleanup(func() {
		common.DashboardDataRateLimitEnable = previousEnabled
		common.DashboardDataRateLimitNum = previousLimit
		common.DashboardDataRateLimitDuration = previousDuration
	})

	gin.SetMode(gin.TestMode)
	capacityFirst := gin.New()
	capacityFirst.Use(func(c *gin.Context) { c.Set("id", 4505) })
	capacityFirst.GET("/capacity", RateLimitCapacityGate(), func(c *gin.Context) { c.Status(http.StatusOK) })
	capacityFirst.GET("/data/self", DashboardDataRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for requestNumber := 1; requestNumber <= rateLimitCapacityMaxRequests; requestNumber++ {
		recorder := performRateLimitCapacityRequest(capacityFirst, "/capacity")
		require.Equalf(t, http.StatusOK, recorder.Code, "capacity request %d", requestNumber)
	}
	assert.Equal(t, http.StatusServiceUnavailable, performRateLimitCapacityRequest(capacityFirst, "/capacity").Code)
	assert.Equal(t, http.StatusOK, performRateLimitCapacityRequest(capacityFirst, "/data/self").Code)

	dashboardFirst := gin.New()
	dashboardFirst.Use(func(c *gin.Context) { c.Set("id", 4606) })
	dashboardFirst.GET("/capacity", RateLimitCapacityGate(), func(c *gin.Context) { c.Status(http.StatusOK) })
	dashboardFirst.GET("/data/self", DashboardDataRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	assert.Equal(t, http.StatusOK, performRateLimitCapacityRequest(dashboardFirst, "/data/self").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitCapacityRequest(dashboardFirst, "/data/self").Code)
	assert.Equal(t, http.StatusOK, performRateLimitCapacityRequest(dashboardFirst, "/capacity").Code)
}

func TestRateLimitCapacityGateMemoryBackendUsesRetryableResponse(t *testing.T) {
	previousEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousEnabled })
	require.NoError(t, i18n.Init())
	userID := int(rateLimitCapacityMemoryUserID.Add(1)) + 1_000_000
	engine, handled := newRateLimitCapacityTestEngine(userID, "/capacity", RateLimitCapacityGate())

	for requestNumber := 1; requestNumber <= rateLimitCapacityMaxRequests; requestNumber++ {
		recorder := performRateLimitCapacityRequest(engine, "/capacity")
		require.Equalf(t, http.StatusOK, recorder.Code, "request %d", requestNumber)
	}
	rejected := performRateLimitCapacityRequest(engine, "/capacity")

	assertRateLimitCapacityThrottleResponse(t, rejected, userID)
	assert.Equal(t, rateLimitCapacityMaxRequests, *handled)
}
