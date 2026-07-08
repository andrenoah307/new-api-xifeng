package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type PressureCoolingState struct {
	State         string // "obs" | "cool" | "susp"
	Violations    int64
	TotalRequests int64
	WindowStart   int64
	CooldownUntil int64
	Consecutive   int64
	GraceUntil    int64
}

var pressureCoolingMemStore sync.Map

func pressureCoolingRedisKey(channelId int) string {
	return fmt.Sprintf("pc:state:%d", channelId)
}

func loadPressureCoolingState(channelId int) *PressureCoolingState {
	if common.RedisEnabled {
		return loadPressureCoolingStateRedis(channelId)
	}
	return loadPressureCoolingStateMemory(channelId)
}

func savePressureCoolingState(channelId int, state *PressureCoolingState, ttlSeconds int) {
	if common.RedisEnabled {
		savePressureCoolingStateRedis(channelId, state, ttlSeconds)
	} else {
		savePressureCoolingStateMemory(channelId, state)
	}
}

func deletePressureCoolingState(channelId int) {
	if common.RedisEnabled {
		common.RDB.Del(context.Background(), pressureCoolingRedisKey(channelId))
	}
	pressureCoolingMemStore.Delete(channelId)
}

func loadPressureCoolingStateRedis(channelId int) *PressureCoolingState {
	ctx := context.Background()
	key := pressureCoolingRedisKey(channelId)
	vals, err := common.RDB.HGetAll(ctx, key).Result()
	if err != nil || len(vals) == 0 {
		return &PressureCoolingState{State: "obs"}
	}
	s := &PressureCoolingState{}
	s.State = vals["st"]
	if s.State == "" {
		s.State = "obs"
	}
	s.Violations, _ = strconv.ParseInt(vals["vc"], 10, 64)
	s.TotalRequests, _ = strconv.ParseInt(vals["tr"], 10, 64)
	s.WindowStart, _ = strconv.ParseInt(vals["ws"], 10, 64)
	s.CooldownUntil, _ = strconv.ParseInt(vals["cu"], 10, 64)
	s.Consecutive, _ = strconv.ParseInt(vals["cc"], 10, 64)
	s.GraceUntil, _ = strconv.ParseInt(vals["gu"], 10, 64)
	return s
}

func savePressureCoolingStateRedis(channelId int, state *PressureCoolingState, ttlSeconds int) {
	ctx := context.Background()
	key := pressureCoolingRedisKey(channelId)
	fields := map[string]interface{}{
		"st": state.State,
		"vc": strconv.FormatInt(state.Violations, 10),
		"tr": strconv.FormatInt(state.TotalRequests, 10),
		"ws": strconv.FormatInt(state.WindowStart, 10),
		"cu": strconv.FormatInt(state.CooldownUntil, 10),
		"cc": strconv.FormatInt(state.Consecutive, 10),
		"gu": strconv.FormatInt(state.GraceUntil, 10),
	}
	common.RDB.HSet(ctx, key, fields)
	if ttlSeconds > 0 {
		common.RDB.Expire(ctx, key, time.Duration(ttlSeconds)*time.Second)
	}
}

func loadPressureCoolingStateMemory(channelId int) *PressureCoolingState {
	v, ok := pressureCoolingMemStore.Load(channelId)
	if !ok {
		return &PressureCoolingState{State: "obs"}
	}
	s := v.(*PressureCoolingState)
	cp := *s
	return &cp
}

func savePressureCoolingStateMemory(channelId int, state *PressureCoolingState) {
	cp := *state
	pressureCoolingMemStore.Store(channelId, &cp)
}

func listCoolingChannelStates() map[int]*PressureCoolingState {
	result := make(map[int]*PressureCoolingState)
	if common.RedisEnabled {
		ctx := context.Background()
		var cursor uint64
		for {
			keys, next, err := common.RDB.Scan(ctx, cursor, "pc:state:*", 200).Result()
			if err != nil {
				break
			}
			for _, key := range keys {
				var chId int
				if _, err := fmt.Sscanf(key, "pc:state:%d", &chId); err != nil || chId <= 0 {
					continue
				}
				st := loadPressureCoolingStateRedis(chId)
				if st.State == "cool" || st.State == "susp" {
					result[chId] = st
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	pressureCoolingMemStore.Range(func(key, value interface{}) bool {
		chId := key.(int)
		st := value.(*PressureCoolingState)
		if st.State == "cool" || st.State == "susp" {
			if _, exists := result[chId]; !exists {
				cp := *st
				result[chId] = &cp
			}
		}
		return true
	})
	return result
}
