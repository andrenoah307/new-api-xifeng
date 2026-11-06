package model

import (
	"errors"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecoverPressureCoolingChannelPreservesManualStatus(t *testing.T) {
	for _, status := range []int{common.ChannelStatusEnabled, common.ChannelStatusManuallyDisabled, common.ChannelStatusAutoDisabled} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			truncateTables(t)
			ch := Channel{Id: 12701, Status: status, Key: "private-test-key", Name: "keep-name", UsedQuota: 123,
				OtherInfo: `{"unrelated":"keep"}`}
			require.NoError(t, DB.Create(&ch).Error)
			require.NoError(t, DB.Create(&Ability{ChannelId: ch.Id, Group: "pro", Model: "cooling-model", Enabled: false}).Error)
			changed, err := RecoverPressureCoolingChannel(ch.Id, "recovered")
			require.NoError(t, err)
			assert.Equal(t, status == common.ChannelStatusAutoDisabled, changed)
			stored, err := GetChannelById(ch.Id, true)
			require.NoError(t, err)
			expected := status
			if changed {
				expected = common.ChannelStatusEnabled
			}
			assert.Equal(t, expected, stored.Status)
			assert.Equal(t, ch.Key, stored.Key)
			assert.Equal(t, ch.Name, stored.Name)
			assert.Equal(t, ch.UsedQuota, stored.UsedQuota)
			assert.Equal(t, "keep", stored.GetOtherInfo()["unrelated"])
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", ch.Id).First(&ability).Error)
			assert.Equal(t, changed, ability.Enabled)
		})
	}
}

func TestRecoverPressureCoolingChannelRollsBackAbilityFailure(t *testing.T) {
	truncateTables(t)
	ch := Channel{Id: 12702, Status: common.ChannelStatusAutoDisabled, Key: "test-key", Group: "pro", Models: "cooling-model"}
	require.NoError(t, DB.Create(&ch).Error)
	require.NoError(t, DB.Create(&Ability{ChannelId: ch.Id, Group: "pro", Model: "cooling-model", Enabled: false}).Error)
	previous := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previous; InitChannelCache() })
	InitChannelCache()
	const callback = "test:cooling-ability-failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("injected ability failure"))
		}
	}))
	t.Cleanup(func() { DB.Callback().Update().Remove(callback) })
	changed, err := RecoverPressureCoolingChannel(ch.Id, "recovered")
	require.Error(t, err)
	assert.False(t, changed)
	stored, err := GetChannelById(ch.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	cached, err := CacheGetChannel(ch.Id)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, cached.Status)
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", ch.Id).First(&ability).Error)
	assert.False(t, ability.Enabled)
}
