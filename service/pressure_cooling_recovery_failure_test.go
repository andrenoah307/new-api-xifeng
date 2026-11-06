package service

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPressureCoolingRecoveryUsesDatabaseStatus(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	usePressureCoolingRuntimeRedis(t, miniredis.RunT(t))
	configurePressureCooling(t, func(c *operation_setting.PressureCoolingSetting) { c.Enabled = true })
	ch := pressureCoolingTestChannel(t, 12601, "stale-cache", "pro", "recovery-model", dto.ChannelSettings{})
	model.InitChannelCache()
	// Simulate another writer disabling the database while this node still caches enabled.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("status", common.ChannelStatusAutoDisabled).Error)
	now := time.Now().Unix()
	savePressureCoolingState(ch.Id, &PressureCoolingState{State: "cool", CooldownUntil: now + 60, Consecutive: 1}, 900)
	CheckPressureCooling(ch.Id, 100)
	assert.Equal(t, "cool", loadPressureCoolingState(ch.Id).State)
	pressureCoolingRecoveryOnce(now)
	assert.Equal(t, "cool", loadPressureCoolingState(ch.Id).State)
	pressureCoolingRecoveryOnce(now + 61)
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

func TestPressureCoolingRecoveryRetriesFailedStatusWrite(t *testing.T) {
	preparePressureCoolingGroupTest(t)
	usePressureCoolingRuntimeRedis(t, miniredis.RunT(t))
	configurePressureCooling(t, func(c *operation_setting.PressureCoolingSetting) { c.Enabled = true })
	ch := pressureCoolingTestChannel(t, 12602, "failed-write", "pro", "recovery-model", dto.ChannelSettings{})
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(ch.Id, "", common.ChannelStatusAutoDisabled, "cooling"))
	now := time.Now().Unix()
	savePressureCoolingState(ch.Id, &PressureCoolingState{State: "cool", CooldownUntil: now - 1, Consecutive: 1}, 900)
	const callback = "test:reject-cooling-recovery"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(errors.New("injected write failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
	pressureCoolingRecoveryOnce(now)
	assert.Equal(t, "cool", loadPressureCoolingState(ch.Id).State, "failed recovery must remain retryable")
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	pressureCoolingRecoveryOnce(now + 31)
	stored, err = model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, "obs", loadPressureCoolingState(ch.Id).State)
}
