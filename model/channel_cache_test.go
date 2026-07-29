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
