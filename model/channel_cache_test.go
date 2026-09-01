package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetSatisfiedChannelCandidatesSnapshot(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
			require.NoError(t, DB.Exec("DELETE FROM channels").Error)

			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				if originalMemoryCacheEnabled {
					InitChannelCache()
				}
			})
			truncateTables(t)

			highPriority := int64(20)
			lowPriority := int64(10)
			highWeight := uint(100)
			lowWeight := uint(1)
			channels := []*Channel{
				{Id: 9101, Name: "high-a", Key: "key-a", Status: common.ChannelStatusEnabled, Group: "default", Models: "snapshot-model", Priority: &highPriority, Weight: &highWeight},
				{Id: 9102, Name: "high-b", Key: "key-b", Status: common.ChannelStatusEnabled, Group: "default", Models: "snapshot-model", Priority: &highPriority, Weight: &lowWeight},
				{Id: 9103, Name: "low", Key: "key-c", Status: common.ChannelStatusEnabled, Group: "default", Models: "snapshot-model", Priority: &lowPriority, Weight: &lowWeight},
			}
			channels[0].SetSetting(dto.ChannelSettings{RateLimit: &dto.ChannelRateLimit{Enabled: true, RPM: 1, OnLimit: "skip"}})
			for _, channel := range channels {
				require.NoError(t, DB.Create(channel).Error)
				require.NoError(t, DB.Create(&Ability{
					Group:     "default",
					Model:     "snapshot-model",
					ChannelId: channel.Id,
					Enabled:   true,
					Priority:  channel.Priority,
					Weight:    *channel.Weight,
				}).Error)
			}

			if memoryCacheEnabled {
				InitChannelCache()
			}

			channelQueries := 0
			batchQuerySeen := false
			if !memoryCacheEnabled {
				callbackName := "test:satisfied-channel-batch-load"
				require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
					table := tx.Statement.Table
					if tx.Statement.Schema != nil {
						table = tx.Statement.Schema.Table
					}
					if table != "channels" {
						return
					}
					channelQueries++
					batchQuerySeen = batchQuerySeen || strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), " IN ")
				}))
				t.Cleanup(func() {
					DB.Callback().Query().Remove(callbackName)
				})
			}

			candidates, err := GetSatisfiedChannelCandidates("default", "snapshot-model", 0, "")
			require.NoError(t, err)
			require.Len(t, candidates, 2)
			assert.ElementsMatch(t, []int{9101, 9102}, []int{candidates[0].Id, candidates[1].Id})
			var limitedChannel *Channel
			for _, candidate := range candidates {
				if candidate.Id == 9101 {
					limitedChannel = candidate
					break
				}
			}
			require.NotNil(t, limitedChannel)
			require.NotNil(t, limitedChannel.GetSetting().RateLimit)
			if !memoryCacheEnabled {
				assert.Equal(t, 1, channelQueries)
				assert.True(t, batchQuerySeen, "the non-cache path must load complete channels with one WHERE id IN query")
			}

			candidates[0] = nil
			again, err := GetSatisfiedChannelCandidates("default", "snapshot-model", 0, "")
			require.NoError(t, err)
			require.Len(t, again, 2)
			assert.NotNil(t, again[0], "mutating a returned snapshot must not alter the cached candidate slice")

			lowerPriority, err := GetSatisfiedChannelCandidates("default", "snapshot-model", 1, "")
			require.NoError(t, err)
			require.Len(t, lowerPriority, 1)
			assert.Equal(t, 9103, lowerPriority[0].Id)
		})
	}
}

func TestGetSatisfiedChannelCandidatesFiltersCooledGroupInMemoryAndDB(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			truncateTables(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			originalGroups := group2model2channels
			originalChannels := channelsIDM
			common.MemoryCacheEnabled = memoryCacheEnabled
			cooling := map[int]map[string]struct{}{9201: {"pro": {}}}
			SetChannelGroupCooling(cooling)
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				group2model2channels = originalGroups
				channelsIDM = originalChannels
				SetChannelGroupCooling(nil)
			})

			priority := int64(10)
			weight := uint(1)
			channels := []*Channel{
				{Id: 9201, Name: "cooled", Key: "cooled-key", Status: common.ChannelStatusEnabled, Group: "pro", Models: "cooling-model", Priority: &priority, Weight: &weight},
				{Id: 9202, Name: "available", Key: "available-key", Status: common.ChannelStatusEnabled, Group: "pro", Models: "cooling-model", Priority: &priority, Weight: &weight},
			}
			for _, channel := range channels {
				require.NoError(t, DB.Create(channel).Error)
				require.NoError(t, DB.Create(&Ability{Group: "pro", Model: "cooling-model", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
			}
			if memoryCacheEnabled {
				group2model2channels = map[string]map[string][]int{"pro": {"cooling-model": {9201, 9202}}}
				channelsIDM = map[int]*Channel{9201: channels[0], 9202: channels[1]}
			}

			candidates, err := GetSatisfiedChannelCandidates("pro", "cooling-model", 0, "")
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			assert.Equal(t, 9202, candidates[0].Id)
		})
	}
}

func TestCountEnabledChannelsForGroupModelExcludesCooledChannels(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			truncateTables(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			originalGroups := group2model2channels
			originalChannels := channelsIDM
			common.MemoryCacheEnabled = memoryCacheEnabled
			SetChannelGroupCooling(map[int]map[string]struct{}{9301: {"pro": {}}})
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				group2model2channels = originalGroups
				channelsIDM = originalChannels
				SetChannelGroupCooling(nil)
			})

			priority := int64(10)
			weight := uint(1)
			cooled := &Channel{Id: 9301, Name: "cooled-count", Key: "cooled-count-key", Status: common.ChannelStatusEnabled, Group: "pro", Models: "count-model", Priority: &priority, Weight: &weight}
			available := &Channel{Id: 9302, Name: "available-count", Key: "available-count-key", Status: common.ChannelStatusEnabled, Group: "pro", Models: "count-model", Priority: &priority, Weight: &weight}
			if memoryCacheEnabled {
				group2model2channels = map[string]map[string][]int{"pro": {"count-model": {cooled.Id, available.Id}}}
				channelsIDM = map[int]*Channel{cooled.Id: cooled, available.Id: available}
			} else {
				require.NoError(t, DB.Create(&Ability{Group: "pro", Model: "count-model", ChannelId: cooled.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
				require.NoError(t, DB.Create(&Ability{Group: "pro", Model: "count-model", ChannelId: available.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
			}

			assert.Equal(t, 1, CountEnabledChannelsForGroupModel("pro", "count-model"))
		})
	}
}

func TestCacheUpdateChannelStatusEnabledRefillsAllBucketsIdempotently(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		group2model2channels = originalGroups
		channelsIDM = originalChannels
	})

	priority := int64(10)
	weight := uint(1)
	channel := &Channel{
		Id:       9401,
		Name:     "refill",
		Key:      "refill-key",
		Status:   common.ChannelStatusManuallyDisabled,
		Group:    "pro,cheap",
		Models:   "model-a,model-b",
		Priority: &priority,
		Weight:   &weight,
	}
	channelsIDM = map[int]*Channel{channel.Id: channel}
	group2model2channels = map[string]map[string][]int{
		"pro":   nil,
		"cheap": {"model-a": {}, "model-b": {}},
	}

	CacheUpdateChannelStatus(channel.Id, common.ChannelStatusEnabled)
	CacheUpdateChannelStatus(channel.Id, common.ChannelStatusEnabled)

	for _, group := range []string{"pro", "cheap"} {
		for _, modelName := range []string{"model-a", "model-b"} {
			ids := group2model2channels[group][modelName]
			require.Len(t, ids, 1)
			assert.Equal(t, channel.Id, ids[0])
		}
	}
}
