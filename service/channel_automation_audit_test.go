package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAutoDisableEnableAudit(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	ch := pressureCoolingTestChannel(t, 12810, "audit", "pro", "audit-model", dto.ChannelSettings{})
	model.InitChannelCache()
	DisableChannel(*types.NewChannelError(ch.Id, ch.Type, ch.Name, false, ch.Key, true), "Invalid credential "+ch.Key+"; token=secret-value; Bearer another-secret")
	EnableChannel(ch.Id, ch.Key, ch.Name)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("channel_id = ? AND type = ?", ch.Id, model.LogTypeManage).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Contains(t, logs[0].Content, "channel.auto_disable")
	assert.Contains(t, logs[1].Content, "channel.auto_enable")
	for _, log := range logs {
		assert.Zero(t, log.UserId)
		for _, secret := range []string{ch.Key, "secret-value", "another-secret"} {
			assert.NotContains(t, log.Content+log.Other, secret)
		}
	}
}

func TestPressureCoolingChannelConsecutiveLimit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limit      *int
		wantState  string
		wantStatus int
	}{
		{name: "inherit global", wantState: "obs", wantStatus: common.ChannelStatusEnabled},
		{name: "channel limit reached", limit: func() *int { n := 1; return &n }(), wantState: "susp", wantStatus: common.ChannelStatusAutoDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preparePressureCoolingGroupTest(t)
			usePressureCoolingRuntimeRedis(t, miniredis.RunT(t))
			configurePressureCooling(t, func(c *operation_setting.PressureCoolingSetting) { c.Enabled = true; c.MaxConsecutiveCooldowns = 5 })
			ch := pressureCoolingTestChannel(t, 12820, "limit", "pro", "audit-model", dto.ChannelSettings{PressureCooling: &dto.PressureCoolingOverride{MaxConsecutiveCooldowns: tc.limit}})
			model.InitChannelCache()
			require.True(t, model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusAutoDisabled, "cooling"))
			now := time.Now().Unix()
			savePressureCoolingState(ch.Id, &PressureCoolingState{State: "cool", Consecutive: 1, CooldownUntil: now - 1}, 900)
			pressureCoolingRecoveryOnce(now)
			assert.Equal(t, tc.wantState, loadPressureCoolingState(ch.Id).State)
			stored, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, stored.Status)
		})
	}
}

func TestPressureCoolingLifecycleAudit(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	usePressureCoolingRuntimeRedis(t, miniredis.RunT(t))
	configurePressureCooling(t, func(c *operation_setting.PressureCoolingSetting) {
		c.Enabled = true
		c.MinActiveChannelsPerGroup = 1
		c.CooldownSeconds = 60
		c.MaxCooldownSeconds = 120
	})
	ch := pressureCoolingTestChannel(t, 12811, "cooling-audit", "pro", "audit-model", dto.ChannelSettings{})
	pressureCoolingTestChannel(t, 12812, "peer", "pro", "audit-model", dto.ChannelSettings{})
	model.InitChannelCache()
	now := time.Now().Unix()
	executePressureCooling(ch, &PressureCoolingState{State: "obs", Violations: 3, TotalRequests: 3}, resolvePressureCoolingConfig(nil), now, 360, nil)
	pressureCoolingRecoveryOnce(now + 61)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("channel_id = ? AND type = ?", ch.Id, model.LogTypeManage).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Contains(t, logs[0].Content, "channel.pressure_cooling.cooldown")
	assert.Contains(t, logs[0].Content, "3/3")
	assert.Contains(t, logs[0].Other, `"status_updated":true`)
	assert.Contains(t, logs[0].Other, `"marker_saved":true`)
	assert.Contains(t, logs[1].Content, "channel.pressure_cooling.recovered")
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	state := loadPressureCoolingState(ch.Id)
	for i := 0; i < 2; i++ {
		recordPressureCoolingAudit(ch.Id, "recovery_failed", "database_read_failed", state, resolvePressureCoolingConfig(nil))
	}
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("channel_id = ? AND type = ?", ch.Id, model.LogTypeManage).Count(&count).Error)
	assert.EqualValues(t, 3, count, "identical recovery failures are rate limited")
}
