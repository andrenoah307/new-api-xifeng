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
	UserRPM   int            `json:"user_rpm,omitempty"`
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

func t3SetModelNameRPMAcquireSpy(t *testing.T, fn func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result) *int {
	t.Helper()
	previous := modelNameRPMAcquire
	calls := 0
	modelNameRPMAcquire = func(ctx context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		calls++
		return fn(ctx, buckets)
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
	db := t3SetupChannelDB(t)
	dbQueries := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("t3:count_unmatched_queries", func(*gorm.DB) {
		dbQueries++
	}))
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
		t.Fatal("Acquire must not be called for an unmatched model")
		return model_name_limiter.Result{}
	})

	require.True(t, enforceModelNameRPM(c, "unconfigured-model", "free", c.Request.URL.Path))
	assert.Equal(t, 0, *calls)
	assert.Equal(t, 0, dbQueries)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMBuildsGlobalGroupAndUserBuckets(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"free": 3}},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:free", Limit: 3, Scope: "group"},
			{Key: "mdrl:v1:rpm:user:gpt-4o:42", Limit: 2, Scope: "user"},
		}, buckets)
		return model_name_limiter.Result{Allowed: true}
	})

	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
	assert.Equal(t, 1, *calls)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyModelNameRPMChecked))
}

func TestT3ModelNameRPMMissingUserIDSkipsOnlyUserBucket(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"free": 3}},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:free", Limit: 3, Scope: "group"},
		}, buckets)
		for _, bucket := range buckets {
			assert.NotContains(t, bucket.Key, ":user:")
			assert.NotContains(t, bucket.Key, ":0")
		}
		return model_name_limiter.Result{Allowed: true}
	})

	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
}

func TestT3ModelNameRPMIsIdempotentAcrossEntries(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/videos")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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

func TestT3ModelNameRPMUserRejectionUsesDistinctRedactedMessage(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"secret-model": {GlobalRPM: 17, UserRPM: 13, GroupRPM: map[string]int{"secret-group": 14}},
	})

	globalContext, globalRecorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	globalContext.Set(string(constant.ContextKeyLanguage), i18n.LangZhCN)
	common.SetContextKey(globalContext, constant.ContextKeyUserId, 42)
	rejectScope := "global"
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
		if rejectScope == "user" {
			return model_name_limiter.Result{Allowed: false, Scope: "user", Limit: 13, Current: 13}
		}
		return model_name_limiter.Result{Allowed: false, Scope: rejectScope, Limit: 17, Current: 17}
	})
	require.False(t, enforceModelNameRPM(globalContext, "secret-model", "secret-group", globalContext.Request.URL.Path))

	userContext, userRecorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	userContext.Set(string(constant.ContextKeyLanguage), i18n.LangZhCN)
	common.SetContextKey(userContext, constant.ContextKeyUserId, 42)
	rejectScope = "user"
	require.False(t, enforceModelNameRPM(userContext, "secret-model", "secret-group", userContext.Request.URL.Path))

	assert.Equal(t, http.StatusServiceUnavailable, userRecorder.Code)
	assert.Contains(t, globalRecorder.Body.String(), "模型请求过于频繁，请稍后重试")
	assert.Contains(t, userRecorder.Body.String(), "你对该模型的请求过于频繁，请稍后重试")
	assert.NotEqual(t, globalRecorder.Body.String(), userRecorder.Body.String())
	for _, secret := range []string{"secret-model", "secret-group", "17", "14", "13", "global", "user"} {
		assert.NotContains(t, userRecorder.Body.String(), secret)
	}
}

func TestT3ModelNameRPMFailOpenMarksRequestChecked(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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
	t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"}}, buckets)
		return model_name_limiter.Result{Allowed: true}
	})
	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))
}

// Global RPM 0 means unlimited: the global bucket must still be sent (count-only)
// and the user/group sub-limits must still reject with the standard 503 shape.
func TestT3ModelNameRPMUnlimitedGlobalStillCountsAndEnforcesSubLimits(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 0, UserRPM: 2, GroupRPM: map[string]int{"free": 3}},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 0, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:free", Limit: 3, Scope: "group"},
			{Key: "mdrl:v1:rpm:user:gpt-4o:42", Limit: 2, Scope: "user"},
		}, buckets)
		return model_name_limiter.Result{Allowed: true}
	})
	require.True(t, enforceModelNameRPM(c, "gpt-4o", "free", c.Request.URL.Path))

	rejected, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	rejected.Set(string(constant.ContextKeyLanguage), i18n.LangZhCN)
	common.SetContextKey(rejected, constant.ContextKeyUserId, 42)
	t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "user", Limit: 2, Current: 2}
	})
	require.False(t, enforceModelNameRPM(rejected, "gpt-4o", "free", rejected.Request.URL.Path))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "60", recorder.Header().Get("Retry-After"))
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, string(types.ErrorCodeModelNameRateLimited), response.Error.Code)
}

func TestT3E1SpecifiedChannelBranchCountsRPM(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"default": 4}},
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
	common.SetContextKey(c, constant.ContextKeyUserId, 71)
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channelID))
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:default", Limit: 4, Scope: "group"},
			{Key: "mdrl:v1:rpm:user:gpt-4o:71", Limit: 2, Scope: "user"},
		}, buckets)
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 10, Current: 10}
	})

	Distribute()(c)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestT3E2OrdinaryBranchCountsBeforeChannelSelection(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"free": 4}},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, 72)
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:free", Limit: 4, Scope: "group"},
			{Key: "mdrl:v1:rpm:user:gpt-4o:72", Limit: 2, Scope: "user"},
		}, buckets)
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 10, Current: 10}
	})

	Distribute()(c)
	assert.Equal(t, 1, *calls)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestT3E2PlaygroundUsesAuthorizedGroup(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 10, UserRPM: 2, GroupRPM: map[string]int{"vip": 3}},
	})
	c, recorder := t3NewModelNameRPMTestContext(t, "/pg/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o","group":"vip"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserId, 73)
	calls := t3SetModelNameRPMAcquireSpy(t, func(_ context.Context, buckets []model_name_limiter.Bucket) model_name_limiter.Result {
		assert.Equal(t, []model_name_limiter.Bucket{
			{Key: "mdrl:v1:rpm:model:gpt-4o", Limit: 10, Scope: "global"},
			{Key: "mdrl:v1:rpm:group:gpt-4o:vip", Limit: 3, Scope: "group"},
			{Key: "mdrl:v1:rpm:user:gpt-4o:73", Limit: 2, Scope: "user"},
		}, buckets)
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
	calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
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
			calls := t3SetModelNameRPMAcquireSpy(t, func(context.Context, []model_name_limiter.Bucket) model_name_limiter.Result {
				t.Fatal("fetch and notify routes must not acquire model-name RPM buckets")
				return model_name_limiter.Result{}
			})
			Distribute()(c)
			assert.Equal(t, 0, *calls)
			_, marked := c.Get(string(constant.ContextKeyModelNameRPMChecked))
			assert.False(t, marked)
		})
	}
}
