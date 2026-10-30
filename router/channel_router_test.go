package router

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelStatusRoutesUseOperatePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelOperate, controller.UpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/status/batch", authz.ChannelOperate, controller.BatchUpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
}

func TestPressureCoolingRuntimeRouteUsesReadPermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/pressure_cooling/runtime", authz.ChannelRead, controller.GetPressureCoolingRuntime)
}

func TestChannelDeleteRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite, controller.DeleteChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/batch", authz.ChannelSensitiveWrite, controller.DeleteChannelBatch)
	assertChannelRoutePermission(t, http.MethodDelete, "/disabled", authz.ChannelSensitiveWrite, controller.DeleteDisabledChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/tag", authz.ChannelWrite, controller.EditTagChannels)
	assertChannelRoutePermission(t, http.MethodPost, "/batch/tag", authz.ChannelWrite, controller.BatchSetChannelTag)
}

func TestChannelInflightRoutesSeparateReadFromOperate(t *testing.T) {
	// Watching the panel is a read; tearing connections down is an operation.
	assertChannelRoutePermission(t, http.MethodGet, "/:id/inflight", authz.ChannelRead, controller.GetChannelInflight)
	assertChannelRoutePermission(t, http.MethodGet, "/inflight/runtime", authz.ChannelRead, controller.GetChannelInflightRuntime)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/inflight/cleanup", authz.ChannelOperate, controller.CleanupChannelInflight)
}

func TestChannelCacheRateRouteUsesReadPermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/cache-rate", authz.ChannelRead, controller.GetChannelCacheRates)
}

func TestChannelStatusRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})
}

func TestChannelCollectionPathsReachAuthenticationWithCostSibling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("channel-route-test"))))
	SetApiRouter(engine)
	engine.NoRoute(controller.RelayNotFound)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		for _, path := range []string{"/api/channel", "/api/channel/"} {
			t.Run(method+path, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest(method, path+"?p=1&page_size=20", nil))
				assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
				assert.Empty(t, recorder.Header().Get("Location"), "collection requests must not depend on redirects")
				assert.NotContains(t, recorder.Body.String(), "Invalid URL")
			})
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, "/api/channel-cost/1", nil))
		assert.Equal(t, http.StatusUnauthorized, recorder.Code, "cost routes must remain protected")
	}
}

func assertChannelRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}
