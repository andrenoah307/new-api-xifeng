package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertExcludingChannel seeds one enabled channel that refuses the given user
// groups, mirroring what the console writes into channels.setting.
func insertExcludingChannel(t *testing.T, id int, name, group, modelName string, excluded []string) {
	t.Helper()
	priority := int64(10)
	weight := uint(0)
	channel := &model.Channel{
		Id:       id,
		Name:     name,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Group:    group,
		Models:   modelName,
		Priority: &priority,
		Weight:   &weight,
	}
	channel.SetSetting(dto.ChannelSettings{ExcludedUserGroups: excluded})
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func useAutoGroups(t *testing.T, groups ...string) {
	t.Helper()
	original := append([]string(nil), setting.GetAutoGroups()...)
	encoded, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(encoded)))
	t.Cleanup(func() {
		if restored, err := common.Marshal(original); err == nil {
			_ = setting.UpdateAutoGroupsByJsonString(string(restored))
		}
	})
}

// A user group excluded from every channel of the first auto group must fall
// through to the next group instead of failing the request.
func TestCacheGetRandomSatisfiedChannel_AutoGroupSkipsExcludedUserGroup(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memoryCacheEnabled)
			useAutoGroups(t, "default", "vip")

			modelName := "exclusion-auto-model"
			insertExcludingChannel(t, 9601, "loss-making", "default", modelName, []string{"vip"})
			insertExcludingChannel(t, 9602, "vip-only", "vip", modelName, nil)
			if memoryCacheEnabled {
				model.InitChannelCache()
			}

			c := newSatisfiedChannelContext()
			common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
			selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
				Ctx:        c,
				TokenGroup: "auto",
				ModelName:  modelName,
				Retry:      common.GetPointer(0),
			})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 9602, selected.Id)
			assert.Equal(t, "vip", selectedGroup)
		})
	}
}

// The exclusion must apply even when the caller leaves RetryParam.UserGroup
// empty: it is resolved from the request context, so no call site can silently
// re-open an excluded route by forgetting to populate it.
func TestCacheGetRandomSatisfiedChannel_ResolvesUserGroupFromContext(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memoryCacheEnabled)

			modelName := "exclusion-context-model"
			insertExcludingChannel(t, 9611, "loss-making", "default", modelName, []string{"vip"})
			if memoryCacheEnabled {
				model.InitChannelCache()
			}

			excluded := newSatisfiedChannelContext()
			common.SetContextKey(excluded, constant.ContextKeyUserGroup, "vip")
			param := &RetryParam{
				Ctx:        excluded,
				TokenGroup: "default",
				ModelName:  modelName,
				Retry:      common.GetPointer(0),
			}
			selected, _, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			assert.Nil(t, selected, "an excluded user group must not reach the channel")
			assert.Equal(t, "vip", param.UserGroup, "the user group must be resolved from the context")

			allowed := newSatisfiedChannelContext()
			common.SetContextKey(allowed, constant.ContextKeyUserGroup, "default")
			selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{
				Ctx:        allowed,
				TokenGroup: "default",
				ModelName:  modelName,
				Retry:      common.GetPointer(0),
			})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 9611, selected.Id)
		})
	}
}
