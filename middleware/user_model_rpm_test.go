package middleware

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeRecordsBeforeSpecificChannelRPMRejection(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 1},
	})
	db := t3SetupChannelDB(t)
	require.NoError(t, db.Create(&model.Channel{
		Id:     940101,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "test-key",
		Name:   "user-rpm-specific",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o",
	}).Error)
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "mount-specific")
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "940101")

	var records int
	var recordedUser int
	var requestID, modelName string
	previousRecord := recordUserModelRPM
	recordUserModelRPM = func(_ context.Context, userID int, id, model string) error {
		records++
		recordedUser = userID
		requestID, modelName = id, model
		return nil
	}
	t.Cleanup(func() { recordUserModelRPM = previousRecord })

	// The model-name spy is installed below without coupling this test to the
	// user RPM backend's storage representation.
	previousAcquire := modelNameRPMAcquire
	modelNameRPMAcquire = func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 1, Current: 1}
	}
	t.Cleanup(func() { modelNameRPMAcquire = previousAcquire })

	Distribute()(c)
	assert.Equal(t, 1, records)
	assert.Equal(t, 42, recordedUser)
	assert.Equal(t, "mount-specific", requestID)
	assert.Equal(t, "gpt-4o", modelName)
}

func TestDistributeRecordsBeforeOrdinaryRPMRejection(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 1},
	})
	c, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, 43)

	recorded := 0
	previousRecord := recordUserModelRPM
	recordUserModelRPM = func(_ context.Context, userID int, requestID, model string) error {
		recorded++
		assert.Equal(t, 43, userID)
		assert.Equal(t, "rpm-test-request", requestID)
		assert.Equal(t, "gpt-4o", model)
		return nil
	}
	t.Cleanup(func() { recordUserModelRPM = previousRecord })
	previousAcquire := modelNameRPMAcquire
	modelNameRPMAcquire = func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 1, Current: 1}
	}
	t.Cleanup(func() { modelNameRPMAcquire = previousAcquire })

	Distribute()(c)
	assert.Equal(t, 1, recorded)
}

func TestDistributeSkipsUserRPMRecordWhenRequestIDOrModelIsMissing(t *testing.T) {
	t3ConfigureModelNameRPMTest(t, true, map[string]t3ModelNameRPMRule{
		"gpt-4o": {GlobalRPM: 1},
	})
	previousRecord := recordUserModelRPM
	count := 0
	recordUserModelRPM = func(context.Context, int, string, string) error {
		count++
		return nil
	}
	t.Cleanup(func() { recordUserModelRPM = previousRecord })
	previousAcquire := modelNameRPMAcquire
	modelNameRPMAcquire = func(context.Context, []string, []int) model_name_limiter.Result {
		return model_name_limiter.Result{Allowed: false, Scope: "global", Limit: 1, Current: 1}
	}
	t.Cleanup(func() { modelNameRPMAcquire = previousAcquire })

	valid, _ := t3NewModelNameRPMTestContext(t, "/v1/chat/completions")
	valid.Request.Body = io.NopCloser(strings.NewReader(`{"model":"gpt-4o"}`))
	valid.Request.Header.Set("Content-Type", "application/json")
	valid.Set(common.RequestIdKey, "")
	common.SetContextKey(valid, constant.ContextKeyUserId, 7)
	// The request-id middleware is the sole source of IDs; an absent value is
	// intentionally not replaced by a generated identifier.
	Distribute()(valid)
	assert.Equal(t, 0, count)

	fetch, _ := t3NewModelNameRPMTestContext(t, "/mj/task-1/fetch")
	fetch.Request.Method = http.MethodPost
	fetch.Request.Body = io.NopCloser(strings.NewReader(`{}`))
	common.SetContextKey(fetch, constant.ContextKeyUserId, 7)
	Distribute()(fetch)
	assert.Equal(t, 0, count)
}
