package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type t3ModelNameRPMRule struct {
	GlobalRPM int            `json:"global_rpm"`
	GroupRPM  map[string]int `json:"group_rpm,omitempty"`
}

func t3ConfigureModelNameRPMTest(t *testing.T, enabled bool, models map[string]t3ModelNameRPMRule) {
	t.Helper()
	previous := setting.ModelNameRPMRateLimit2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(previous))
	})
	payload, err := common.Marshal(struct {
		Enabled bool                          `json:"enabled"`
		Models  map[string]t3ModelNameRPMRule `json:"models"`
	}{Enabled: enabled, Models: models})
	require.NoError(t, err)
	require.NoError(t, setting.UpdateModelNameRPMRateLimitByJSONString(string(payload)))
	require.NoError(t, i18n.Init())
}

func t3NewModelNameRPMTestContext(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Set(common.RequestIdKey, "rpm-test-request")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "free")
	return c, recorder
}

func t3SetModelNameRPMAcquireSpy(t *testing.T, fn func(context.Context, []string, []int) model_name_limiter.Result) *int {
	t.Helper()
	previous := modelNameRPMAcquire
	calls := 0
	modelNameRPMAcquire = func(ctx context.Context, keys []string, limits []int) model_name_limiter.Result {
		calls++
		return fn(ctx, keys, limits)
	}
	t.Cleanup(func() { modelNameRPMAcquire = previous })
	return &calls
}

func t3SetupChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.MemoryCacheEnabled = oldMemoryCache
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func TestT3ModelNameRPMUnmatchedDoesNotAcquire(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"configured-model": {GlobalRPM: 10},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		t.Fatal("Acquire must not be called for an unmatched model")
		return model_name_limiter.Result{}
	})

	require.True(t, enforceModelNameRPM(c, "unconfigured-model", "free", c.Request.URL.Path))
	assert.Equal(t, 0, *calls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMBuildsGlobalAndGroupKeys(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"free": 3}},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, keys []string, limits []int) model_name_limiter.Result {
		assert.Equal(t, []string{
			"mdrl:v1:rpm:model:gpt-4o",
			"mdrl:v1:rpm:group:gpt-4o:free",
		}, keys)
		assert.Equal(t, []int{10, 3}, limits)
		return model_name_limiter.Result{Allowed: true}
	})

	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
	assert.Equal(t, 1, *calls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMIsIdempotentAcrossEntries(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/videos")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 10, Current: 10}
	})

	require.False(t, enforceModelNameRPM(c, "gpt-4o", "free", "/first"))
	// A later entry sees the marker and must not consume or reject again.
	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", "/second"))
	assert.Equal(t, 1, *calls)
}

func TestT3ModelNameRPMRejectsWithRedactedOpenAIResponse(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"secret-model": {GlobalRPM: 17, GroupRPM: map[string]int{"secret-group": 4}},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Set(string(constant.ContextKeyLanguage), i18n.LangZhCN)
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "group", Limit: 4, Current: 4}
	})

	require.False(t, enforceModelNameRPM(c, "secret-model", "secret-group", c.Request.URL.Path))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, string(types.ErrorCodeModelNameRateLimited), response.Error.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "模型请求过于频繁，请稍后重试")
	assert.NotContains(t, body, "secret-model")
	assert.NotContains(t, body, "secret-group")
	assert.NotContains(t, body, "17")
	assert.NotContains(t, body, "4")
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMFailOpenMarksRequestChecked(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		// T2 represents Redis failures as an allowed result.
		return model_name_limiter.Result{Allowed: true}
	})

	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
	assert.Equal(t, 1, *calls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
	assert.Equal(t, 200, recorder.Code)
}

func TestT3ModelNameRPMTaskResponseUsesTaskShape(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"remix-model": {GlobalRPM: 2},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/videos/origin/remix")
	c.Set(string(constant.ContextKeyLanguage), i18n.LangEn)
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 2, Current: 2}
	})

	require.False(t, EnforceModelNameRPMForTask(c, "remix-model", "free", c.Request.URL.Path))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
	var response dto.TaskError
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, string(types.ErrorCodeModelNameRateLimited), response.Code)
	assert.Equal(t, "Too many requests for this model. Please try again later.", response.Message)
	assert.NotContains(t, recorder.Body.String(), "remix-model")
	assert.NotContains(t, recorder.Body.String(), "free")
	assert.NotContains(t, recorder.Body.String(), "2")
}

func TestT3ModelNameRPMAlreadyCheckedSkipsAcquire(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyModelNameRPMChecked, true)
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		t.Fatal("an already checked request must not acquire")
		return model_name_limiter.Result{}
	})

	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", "/retry"))
	require.True(t, EnforceModelNameRPMForTask(c, "gpt-4o", "free", "/task"))
	assert.Equal(t, 0, *calls)
}

func TestT3ModelNameRPMUsesGlobalOnlyWhenNoGroupRule(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, keys []string, limits []int) model_name_limiter.Result {
		assert.Equal(t, []string{"mdrl:v1:rpm:model:gpt-4o"}, keys)
		assert.Equal(t, []int{10}, limits)
		return model_name_limiter.Result{Allowed: true}
	})
	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
}

func TestT3E1SpecifiedChannelBranchCountsRPM(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	db := t3SetupChannelDB(t)
	channelID := 940001
	require.NoError(t, db.Create(&model.Channel{
		Id:     channelID,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "rpm-e1",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o",
	}).Error)
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channelID))
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, keys []string, limits []int) model_name_limiter.Result {
		assert.Equal(t, []string{"mdrl:v1:rpm:model:gpt-4o"}, keys)
		assert.Equal(t, []int{10}, limits)
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 10, Current: 10}
	})

	Distribute()(c)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestT3E2OrdinaryBranchCountsBeforeChannelSelection(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, keys []string, limits []int) model_name_limiter.Result {
		assert.Equal(t, []string{"mdrl:v1:rpm:model:gpt-4o"}, keys)
		assert.Equal(t, []int{10}, limits)
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 10, Current: 10}
	})

	Distribute()(c)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestT3E2PlaygroundUsesAuthorizedGroup(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, GroupRPM: map[string]int{"vip": 3}},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/pg/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o","group":"vip"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, keys []string, limits []int) model_name_limiter.Result {
		assert.Equal(t, []string{
			"mdrl:v1:rpm:model:gpt-4o",
			"mdrl:v1:rpm:group:gpt-4o:vip",
		}, keys)
		assert.Equal(t, []int{10, 3}, limits)
		return model_name_limiter.Result{Allowed: false, Scope: "group", Limit: 3, Current: 3}
	})

	Distribute()(c)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestT3E2PlaygroundRejectsUnauthorizedGroupBeforeRPM(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/pg/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o","group":"not-usable"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []string, []int) model_name_limiter.Result {
		t.Fatal("unauthorized playground group must not reach the RPM gate")
		return model_name_limiter.Result{}
	})

	Distribute()(c)
	assert.Equal(t, 0, *calls)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestT3FetchModesLeaveRPMUnchecked(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "video get", method: http.MethodGet, path: "/v1/videos/video-1"},
		{name: "video generations get", method: http.MethodGet, path: "/v1/video/generations/video-1"},
		{name: "suno fetch", method: http.MethodPost, path: "/suno/fetch"},
		{name: "suno fetch by id", method: http.MethodGet, path: "/suno/fetch/song-1"},
		{name: "midjourney notify", method: http.MethodPost, path: "/mj/notify", body: `{}`},
		{name: "midjourney fetch", method: http.MethodPost, path: "/mj/task-1/fetch", body: `{}`},
		{name: "midjourney image seed", method: http.MethodPost, path: "/mj/task-1/image-seed", body: `{}`},
		{name: "jimeng get result mode", method: http.MethodGet, path: "/v1/video/generations/task-1"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c, _ := t3NewModelNameRPMTestContext(t, test.path)
			c.Request.Method = test.method
			c.Request.Body = io.NopCloser(strings.NewReader(test.body))
			if test.name == "jimeng get result mode" {
				c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			}
			_, shouldSelectChannel, err := getModelRequest(c)
			require.NoError(t, err)
			assert.False(t, shouldSelectChannel)
			_, marked := c.Get(string(constant.ContextKeyModelNameRPMChecked))
			assert.False(t, marked)
		})
	}
}
