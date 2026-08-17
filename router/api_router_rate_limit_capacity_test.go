package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
