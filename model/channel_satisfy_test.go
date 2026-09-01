package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIsChannelEnabledForGroupModelMemoryCooling(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	common.MemoryCacheEnabled = true
	const channelID = 1301
	group2model2channels = map[string]map[string][]int{
		"pro":   {"cooling-model": {channelID}},
		"cheap": {"cooling-model": {channelID}},
	}
	channelsIDM = map[int]*Channel{
		channelID: {Id: channelID, Status: common.ChannelStatusEnabled, Group: "pro,cheap", Models: "cooling-model"},
	}
	SetChannelGroupCooling(nil)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		SetChannelGroupCooling(nil)
	})

	guardTests := []struct {
		name      string
		group     string
		modelName string
		channelID int
	}{
		{name: "empty group", group: "", modelName: "cooling-model", channelID: channelID},
		{name: "empty model", group: "pro", modelName: "", channelID: channelID},
		{name: "zero channel", group: "pro", modelName: "cooling-model", channelID: 0},
		{name: "negative channel", group: "pro", modelName: "cooling-model", channelID: -1},
	}
	for _, test := range guardTests {
		t.Run(test.name, func(t *testing.T) {
			assert.False(t, IsChannelEnabledForGroupModel(test.group, test.modelName, test.channelID))
		})
	}

	assert.True(t, IsChannelEnabledForGroupModel("pro", "cooling-model", channelID))
	assert.True(t, IsChannelEnabledForGroupModel("cheap", "cooling-model", channelID))

	SetChannelGroupCooling(map[int]map[string]struct{}{channelID: {"pro": {}}})
	assert.False(t, IsChannelEnabledForGroupModel("pro", "cooling-model", channelID))
	assert.True(t, IsChannelEnabledForGroupModel("cheap", "cooling-model", channelID))

	SetChannelGroupCooling(nil)
	assert.True(t, IsChannelEnabledForGroupModel("pro", "cooling-model", channelID))
}

func TestIsChannelEnabledForGroupModelCoolingSkipsDatabaseLookup(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	common.MemoryCacheEnabled = false
	group2model2channels = map[string]map[string][]int{}
	channelsIDM = map[int]*Channel{}
	SetChannelGroupCooling(map[int]map[string]struct{}{1302: {"pro": {}}})
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		SetChannelGroupCooling(nil)
	})

	queryCount := 0
	callbackName := "test:is-channel-enabled-cooling-before-db"
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCount++
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Query().Remove(callbackName)) })

	assert.False(t, IsChannelEnabledForGroupModel("pro", "cooling-model", 1302))
	assert.Zero(t, queryCount, "a cooled channel must return before the database branch")
}

func TestIsChannelEnabledForGroupModelMemoryModelMatchingAndAnyGroup(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	common.MemoryCacheEnabled = true
	group2model2channels = map[string]map[string][]int{
		"pro": {"gpt-4-gizmo-*": {1303}},
	}
	channelsIDM = map[int]*Channel{1303: {Id: 1303, Status: common.ChannelStatusEnabled}}
	SetChannelGroupCooling(nil)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		SetChannelGroupCooling(nil)
	})

	assert.True(t, IsChannelEnabledForGroupModel("pro", "gpt-4-gizmo-preview", 1303))
	assert.False(t, IsChannelEnabledForGroupModel("pro", "missing-model", 1303))
	assert.False(t, IsChannelEnabledForGroupModel("missing-group", "gpt-4-gizmo-preview", 1303))
	assert.True(t, IsChannelEnabledForAnyGroupModel([]string{"missing-group", "pro"}, "gpt-4-gizmo-preview", 1303))
	assert.False(t, IsChannelEnabledForAnyGroupModel(nil, "gpt-4-gizmo-preview", 1303))
	assert.False(t, IsChannelEnabledForAnyGroupModel([]string{"missing-group"}, "gpt-4-gizmo-preview", 1303))

	group2model2channels = nil
	assert.False(t, IsChannelEnabledForGroupModel("pro", "gpt-4-gizmo-preview", 1303))
}

func TestIsChannelEnabledForGroupModelDatabasePathAndModelMatching(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCacheEnabled })

	const channelID = 1304
	require.NoError(t, DB.Create(&Ability{Group: "db-pro", Model: "gpt-4-gizmo-*", ChannelId: channelID, Enabled: true}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error)
	})

	assert.True(t, IsChannelEnabledForGroupModel("db-pro", "gpt-4-gizmo-preview", channelID))
	assert.False(t, IsChannelEnabledForGroupModel("db-pro", "missing-model", channelID))

	const exactChannelID = 1305
	require.NoError(t, DB.Create(&Ability{Group: "db-pro", Model: "exact-model", ChannelId: exactChannelID, Enabled: true}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("channel_id = ?", exactChannelID).Delete(&Ability{}).Error)
	})
	assert.True(t, IsChannelEnabledForGroupModel("db-pro", "exact-model", exactChannelID))
}
