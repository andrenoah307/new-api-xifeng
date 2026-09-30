package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRoutingLogsAndMonitoringKeepOriginalGroup(t *testing.T) {
	truncateTables(t)
	oldHook := common.GroupMonitoringHook
	t.Cleanup(func() { common.GroupMonitoringHook = oldHook })
	var monitorGroup string
	common.GroupMonitoringHook = func(group string, _ int, _ bool, _, _, _, _ int, _ string, _ int, _ string) { monitorGroup = group }
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("username", "routing-test")
	common.SetContextKey(c, constant.ContextKeyUserRouting, &dto.UserRoutingInfo{SourceGroup: "original", TargetGroup: "destination", Model: "model-a"})
	RecordConsumeLog(c, 75, RecordConsumeLogParams{Group: "original", ChannelId: 9306, ModelName: "model-a", Quota: 42, PromptTokens: 10})
	RecordErrorLog(c, 75, 9306, "model-a", "", "upstream unavailable", 0, 0, false, "original", nil)
	assert.Equal(t, "original", monitorGroup)
	var logs []*Log
	require.NoError(t, LOG_DB.Where("user_id = ?", 75).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, 42, logs[0].Quota)
	for _, log := range logs {
		assert.Equal(t, "original", log.Group)
		assert.Contains(t, log.Other, `"target_group":"destination"`)
	}
	formatUserLogs(logs, 0)
	for _, log := range logs {
		assert.NotContains(t, log.Other, "user_routing")
		assert.NotContains(t, log.Other, "destination")
		assert.Equal(t, "original", log.Group)
	}
}

func TestUserRoutingPolicyRefreshRetainsSnapshotOnFailure(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserRoutingPolicy{}))
	rules := []UserRoutingRule{{TargetGroup: "destination", Enabled: true}}
	require.NoError(t, SaveUserRoutingRules(76, rules))
	t.Cleanup(func() { require.NoError(t, SaveUserRoutingRules(76, nil)) })
	require.NoError(t, RefreshUserRoutingPolicies())
	assert.Equal(t, rules, CachedUserRoutingRules(76))
	require.NoError(t, DB.Model(&UserRoutingPolicy{}).Where("user_id = ?", 76).Update("rules", "invalid JSON").Error)
	require.Error(t, RefreshUserRoutingPolicies())
	assert.Equal(t, rules, CachedUserRoutingRules(76))
	require.NoError(t, SaveUserRoutingRules(76, nil))
	assert.Empty(t, CachedUserRoutingRules(76))
}
