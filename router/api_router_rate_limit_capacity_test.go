package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type routeRedisCommandCounter struct {
	count atomic.Int64
}

func (c *routeRedisCommandCounter) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	c.count.Add(1)
	return ctx, nil
}

func (*routeRedisCommandCounter) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (c *routeRedisCommandCounter) BeforeProcessPipeline(ctx context.Context, commands []redis.Cmder) (context.Context, error) {
	c.count.Add(int64(len(commands)))
	return ctx, nil
}

func (*routeRedisCommandCounter) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func countRouteDBCommands(t *testing.T, db *gorm.DB) *atomic.Int64 {
	t.Helper()
	var count atomic.Int64
	callbackName := "test:count_route_commands:" + strings.ReplaceAll(t.Name(), "/", "_")
	callback := func(*gorm.DB) { count.Add(1) }
	require.NoError(t, db.Callback().Create().Register(callbackName, callback))
	require.NoError(t, db.Callback().Query().Register(callbackName, callback))
	require.NoError(t, db.Callback().Update().Register(callbackName, callback))
	require.NoError(t, db.Callback().Delete().Register(callbackName, callback))
	require.NoError(t, db.Callback().Row().Register(callbackName, callback))
	require.NoError(t, db.Callback().Raw().Register(callbackName, callback))
	return &count
}

func TestRateLimitCapacityRouteUsesDedicatedGateAfterUserAuth(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	accessToken := "capacity-route-access-token"
	user := &model.User{
		Username:    "capacity-route-user",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &accessToken,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.AutoMigrate(&model.QuotaData{}))

	previousGlobalEnabled := common.GlobalApiRateLimitEnable
	previousDashboardEnabled := common.DashboardDataRateLimitEnable
	previousDashboardLimit := common.DashboardDataRateLimitNum
	previousDashboardDuration := common.DashboardDataRateLimitDuration
	common.GlobalApiRateLimitEnable = false
	common.DashboardDataRateLimitEnable = true
	common.DashboardDataRateLimitNum = 1
	common.DashboardDataRateLimitDuration = 60
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousGlobalEnabled
		common.DashboardDataRateLimitEnable = previousDashboardEnabled
		common.DashboardDataRateLimitNum = previousDashboardLimit
		common.DashboardDataRateLimitDuration = previousDashboardDuration
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("capacity-route-test"))))
	SetApiRouter(engine)

	unauthenticated := httptest.NewRecorder()
	engine.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/rate_limit/capacity", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	request := func() *http.Request {
		request := httptest.NewRequest(http.MethodGet, "/api/rate_limit/capacity", nil)
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("New-Api-User", strconv.Itoa(user.Id))
		return request
	}
	dashboardRequest := func() *http.Request {
		request := request()
		request.URL.Path = "/api/data/self"
		request.URL.RawQuery = "start_timestamp=1&end_timestamp=2"
		return request
	}
	firstDashboard := httptest.NewRecorder()
	engine.ServeHTTP(firstDashboard, dashboardRequest())
	assert.Equal(t, http.StatusOK, firstDashboard.Code)
	secondDashboard := httptest.NewRecorder()
	engine.ServeHTTP(secondDashboard, dashboardRequest())
	assert.Equal(t, http.StatusTooManyRequests, secondDashboard.Code)

	for requestNumber := 1; requestNumber <= 30; requestNumber++ {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request())
		assert.Equalf(t, http.StatusOK, recorder.Code, "capacity request %d", requestNumber)
	}
	rejected := httptest.NewRecorder()
	engine.ServeHTTP(rejected, request())
	assert.Equal(t, http.StatusServiceUnavailable, rejected.Code)
	assert.Equal(t, "60", rejected.Header().Get("Retry-After"))
}

func TestRateLimitCapacityDisabledRouteCommandBudget(t *testing.T) {
	db := setupMonitoringRouterTestDB(t)
	dbCommands := countRouteDBCommands(t, db)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	redisCommands := &routeRedisCommandCounter{}
	redisClient.AddHook(redisCommands)
	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
		require.NoError(t, redisClient.Close())
	})

	previousGlobalRateLimit := common.GlobalApiRateLimitEnable
	previousCardEnabled := setting.IsRateLimitCapacityCardEnabled()
	common.GlobalApiRateLimitEnable = false
	setting.SetRateLimitCapacityCardEnabled(false)
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousGlobalRateLimit
		setting.SetRateLimitCapacityCardEnabled(previousCardEnabled)
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("capacity-command-budget"))))
	engine.GET("/test/session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "capacity-command-user")
		session.Set("role", common.RoleCommonUser)
		session.Set("id", 8801)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	SetApiRouter(engine)

	sessionRecorder := httptest.NewRecorder()
	engine.ServeHTTP(sessionRecorder, httptest.NewRequest(http.MethodGet, "/test/session", nil))
	require.Equal(t, http.StatusNoContent, sessionRecorder.Code)
	sessionResponse := sessionRecorder.Result()
	require.NotEmpty(t, sessionResponse.Cookies())
	sessionCookie := sessionResponse.Cookies()[0]

	dbCommands.Store(0)
	redisCommands.count.Store(0)
	statusRecorder := httptest.NewRecorder()
	engine.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	require.Equal(t, http.StatusOK, statusRecorder.Code)
	assert.Equal(t, int64(0), dbCommands.Load())
	assert.Equal(t, int64(0), redisCommands.count.Load())

	dbCommands.Store(0)
	redisCommands.count.Store(0)
	capacityRequest := httptest.NewRequest(http.MethodGet, "/api/rate_limit/capacity", nil)
	capacityRequest.AddCookie(sessionCookie)
	capacityRequest.Header.Set("New-Api-User", "8801")
	capacityRecorder := httptest.NewRecorder()
	engine.ServeHTTP(capacityRecorder, capacityRequest)
	require.Equal(t, http.StatusOK, capacityRecorder.Code)
	assert.Equal(t, int64(0), dbCommands.Load())
	assert.GreaterOrEqual(t, redisCommands.count.Load(), int64(3))
	assert.LessOrEqual(t, redisCommands.count.Load(), int64(5))
	t.Logf("closed command counts: status db=%d redis=%d; capacity db=%d redis=%d",
		0, 0, dbCommands.Load(), redisCommands.count.Load())
}
