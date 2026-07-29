package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/channel_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepareSatisfiedChannelTest(t *testing.T, memoryCacheEnabled bool) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)

	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = memoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		if originalMemoryCacheEnabled {
			model.InitChannelCache()
		}
	})
	truncate(t)
}

func insertSatisfiedChannel(t *testing.T, id int, name, group, modelName string, weight uint, cfg *dto.ChannelRateLimit) {
	t.Helper()
	priority := int64(10)
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
	if cfg != nil {
		channel.SetSetting(dto.ChannelSettings{RateLimit: cfg})
	}
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

func newSatisfiedChannelContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestCacheGetRandomSatisfiedChannel_ChannelLimitPrefersAvailable(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memoryCacheEnabled)
			modelName := "limit-prefilter-model"
			insertSatisfiedChannel(t, 9201, "saturated", "default", modelName, 100, &dto.ChannelRateLimit{
				Enabled: true,
				RPM:     1,
				OnLimit: "",
			})
			insertSatisfiedChannel(t, 9202, "available", "default", modelName, 0, &dto.ChannelRateLimit{
				Enabled: true,
				RPM:     1,
				OnLimit: channel_limiter.OnLimitSkip,
			})
			if memoryCacheEnabled {
				model.InitChannelCache()
			}

			batchCalls := 0
			param := &RetryParam{
				Ctx:         newSatisfiedChannelContext(),
				TokenGroup:  "default",
				ModelName:   modelName,
				RequestPath: "/v1/chat/completions",
				Retry:       common.GetPointer(0),
				checkBatch: func(_ context.Context, configs map[int]*dto.ChannelRateLimit) map[int]channel_limiter.Decision {
					batchCalls++
					configIDs := make([]int, 0, len(configs))
					for id := range configs {
						configIDs = append(configIDs, id)
					}
					assert.ElementsMatch(t, []int{9201, 9202}, configIDs)
					return map[int]channel_limiter.Decision{
						9201: {Allowed: false, Reason: channel_limiter.ReasonRPMExceeded},
						9202: {Allowed: true},
					}
				},
			}

			selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 9202, selected.Id)
			assert.Equal(t, "default", selectedGroup)
			assert.Equal(t, 1, batchCalls, "all candidate capacity must be read in one batch")
		})
	}
}

func TestCacheGetRandomSatisfiedChannel_ChannelLimitQueueRejectNotFiltered(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		for _, onLimit := range []string{channel_limiter.OnLimitQueue, channel_limiter.OnLimitReject} {
			t.Run(fmt.Sprintf("memory_cache_%t_%s", memoryCacheEnabled, onLimit), func(t *testing.T) {
				prepareSatisfiedChannelTest(t, memoryCacheEnabled)
				modelName := "limit-policy-model"
				insertSatisfiedChannel(t, 9301, "policy-channel", "default", modelName, 100, &dto.ChannelRateLimit{
					Enabled: true,
					RPM:     1,
					OnLimit: onLimit,
				})
				insertSatisfiedChannel(t, 9302, "fallback", "default", modelName, 0, nil)
				if memoryCacheEnabled {
					model.InitChannelCache()
				}

				batchCalls := 0
				selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
					Ctx:        newSatisfiedChannelContext(),
					TokenGroup: "default",
					ModelName:  modelName,
					Retry:      common.GetPointer(0),
					checkBatch: func(context.Context, map[int]*dto.ChannelRateLimit) map[int]channel_limiter.Decision {
						batchCalls++
						return nil
					},
				})
				require.NoError(t, err)
				require.NotNil(t, selected)
				assert.Equal(t, 9301, selected.Id)
				assert.Zero(t, batchCalls, "queue/reject channels must not be capacity-prefiltered")
			})
		}
	}
}

func TestCacheGetRandomSatisfiedChannel_AllChannelLimitsFallback(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memoryCacheEnabled)
			modelName := "all-saturated-model"
			cfg := &dto.ChannelRateLimit{Enabled: true, RPM: 1, OnLimit: channel_limiter.OnLimitSkip}
			insertSatisfiedChannel(t, 9401, "saturated-a", "default", modelName, 0, cfg)
			insertSatisfiedChannel(t, 9402, "saturated-b", "default", modelName, 0, cfg)
			if memoryCacheEnabled {
				model.InitChannelCache()
			}

			batchCalls := 0
			selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
				Ctx:        newSatisfiedChannelContext(),
				TokenGroup: "default",
				ModelName:  modelName,
				Retry:      common.GetPointer(0),
				checkBatch: func(_ context.Context, configs map[int]*dto.ChannelRateLimit) map[int]channel_limiter.Decision {
					batchCalls++
					decisions := make(map[int]channel_limiter.Decision, len(configs))
					for id := range configs {
						decisions[id] = channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonRPMExceeded}
					}
					return decisions
				},
			})
			require.NoError(t, err)
			require.NotNil(t, selected, "all-saturated prefiltering must fall back for authoritative Acquire")
			assert.Contains(t, []int{9401, 9402}, selected.Id)
			assert.Equal(t, 1, batchCalls)
		})
	}
}

func TestCacheGetRandomSatisfiedChannel_AutoGroupChannelLimitDoesNotSwitch(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareSatisfiedChannelTest(t, memoryCacheEnabled)
			originalAutoGroups := append([]string(nil), setting.GetAutoGroups()...)
			autoGroupsJSON, err := common.Marshal([]string{"default", "vip"})
			require.NoError(t, err)
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(autoGroupsJSON)))
			t.Cleanup(func() {
				encoded, err := common.Marshal(originalAutoGroups)
				if err == nil {
					_ = setting.UpdateAutoGroupsByJsonString(string(encoded))
				}
			})

			modelName := "auto-limit-model"
			cfg := &dto.ChannelRateLimit{Enabled: true, RPM: 1, OnLimit: channel_limiter.OnLimitSkip}
			insertSatisfiedChannel(t, 9501, "default-a", "default", modelName, 0, cfg)
			insertSatisfiedChannel(t, 9502, "default-b", "default", modelName, 0, cfg)
			insertSatisfiedChannel(t, 9503, "vip-available", "vip", modelName, 100, nil)
			if memoryCacheEnabled {
				model.InitChannelCache()
			}

			batchCalls := 0
			c := newSatisfiedChannelContext()
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
				Ctx:        c,
				TokenGroup: "auto",
				ModelName:  modelName,
				Retry:      common.GetPointer(0),
				checkBatch: func(_ context.Context, configs map[int]*dto.ChannelRateLimit) map[int]channel_limiter.Decision {
					batchCalls++
					decisions := make(map[int]channel_limiter.Decision, len(configs))
					for id := range configs {
						decisions[id] = channel_limiter.Decision{Allowed: false, Reason: channel_limiter.ReasonRPMExceeded}
					}
					return decisions
				},
			})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Contains(t, []int{9501, 9502}, selected.Id)
			assert.Equal(t, "default", selectedGroup)
			assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
			assert.Equal(t, 1, batchCalls, "saturation must not be mistaken for an empty auto group")
		})
	}
}
