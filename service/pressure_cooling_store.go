package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type PressureCoolingState struct {
	State         string // "obs" | "cool" | "susp"
	Scope         string // "channel" | "groups"
	CooledGroups  []string
	Violations    int64
	TotalRequests int64
	WindowStart   int64
	CooldownUntil int64
	Consecutive   int64
	GraceUntil    int64
}

var pressureCoolingMemStore sync.Map

var errPressureCoolingStateCorrupt = errors.New("pressure cooling state corrupt")

func pressureCoolingRedisKey(channelId int) string {
	return fmt.Sprintf("pc:state:%d", channelId)
}

func loadPressureCoolingState(channelId int) *PressureCoolingState {
	state, err := loadPressureCoolingStateResult(channelId)
	if err != nil || state == nil {
		return &PressureCoolingState{State: "obs", Scope: "channel"}
	}
	return state
}

func loadPressureCoolingStateResult(channelId int) (*PressureCoolingState, error) {
	if common.RedisEnabled {
		return loadPressureCoolingStateRedis(channelId)
	}
	return loadPressureCoolingStateMemory(channelId), nil
}

func savePressureCoolingState(channelId int, state *PressureCoolingState, ttlSeconds int) {
	if common.RedisEnabled {
		savePressureCoolingStateRedis(channelId, state, ttlSeconds)
	} else {
		savePressureCoolingStateMemory(channelId, state)
	}
}

func deletePressureCoolingState(channelId int) {
	if common.RedisEnabled && common.RDB != nil {
		common.RDB.Del(context.Background(), pressureCoolingRedisKey(channelId))
	}
	pressureCoolingMemStore.Delete(channelId)
}

func loadPressureCoolingStateRedis(channelId int) (*PressureCoolingState, error) {
	if common.RDB == nil {
		return nil, fmt.Errorf("pressure cooling redis client is nil")
	}
	ctx := context.Background()
	key := pressureCoolingRedisKey(channelId)
	vals, err := common.RDB.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return &PressureCoolingState{State: "obs", Scope: "channel"}, nil
	}
	s := pressureCoolingStateFromFields(vals)
	fields := []struct {
		name   string
		target *int64
	}{
		{name: "vc", target: &s.Violations},
		{name: "tr", target: &s.TotalRequests},
		{name: "ws", target: &s.WindowStart},
		{name: "cu", target: &s.CooldownUntil},
		{name: "cc", target: &s.Consecutive},
		{name: "gu", target: &s.GraceUntil},
	}
	for _, field := range fields {
		value, ok := vals[field.name]
		if !ok {
			continue
		}
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("pressure cooling state channel %d field %s: %w (%v)", channelId, field.name, errPressureCoolingStateCorrupt, parseErr)
		}
		*field.target = parsed
	}
	return s, nil
}

func savePressureCoolingStateRedis(channelId int, state *PressureCoolingState, ttlSeconds int) {
	if common.RDB == nil {
		return
	}
	ctx := context.Background()
	key := pressureCoolingRedisKey(channelId)
	encoded := pressureCoolingStateFields(state)
	fields := make(map[string]interface{}, len(encoded))
	for field, value := range encoded {
		fields[field] = value
	}
	common.RDB.HSet(ctx, key, fields)
	if ttlSeconds > 0 {
		common.RDB.Expire(ctx, key, time.Duration(ttlSeconds)*time.Second)
	}
}

func loadPressureCoolingStateMemory(channelId int) *PressureCoolingState {
	v, ok := pressureCoolingMemStore.Load(channelId)
	if !ok {
		return &PressureCoolingState{State: "obs", Scope: "channel"}
	}
	s := v.(*PressureCoolingState)
	cp := *s
	if cp.Scope == "" {
		cp.Scope = "channel"
	}
	cp.CooledGroups = append([]string(nil), s.CooledGroups...)
	return &cp
}

func savePressureCoolingStateMemory(channelId int, state *PressureCoolingState) {
	cp := *state
	cp.CooledGroups = append([]string(nil), state.CooledGroups...)
	pressureCoolingMemStore.Store(channelId, &cp)
}

func pressureCoolingStateFields(state *PressureCoolingState) map[string]string {
	scope := ""
	if state.Scope == "groups" {
		scope = "g"
	}
	return map[string]string{
		"st": state.State,
		"vc": strconv.FormatInt(state.Violations, 10),
		"tr": strconv.FormatInt(state.TotalRequests, 10),
		"ws": strconv.FormatInt(state.WindowStart, 10),
		"cu": strconv.FormatInt(state.CooldownUntil, 10),
		"cc": strconv.FormatInt(state.Consecutive, 10),
		"gu": strconv.FormatInt(state.GraceUntil, 10),
		"sc": scope,
		"cg": strings.Join(state.CooledGroups, ","),
	}
}

func pressureCoolingStateFromFields(vals map[string]string) *PressureCoolingState {
	state := &PressureCoolingState{State: vals["st"], Scope: "channel"}
	if state.State == "" {
		state.State = "obs"
	}
	if vals["sc"] == "g" {
		state.Scope = "groups"
	}
	if groups := strings.Split(vals["cg"], ","); len(groups) > 0 && vals["cg"] != "" {
		state.CooledGroups = make([]string, 0, len(groups))
		for _, group := range groups {
			if group = strings.TrimSpace(group); group != "" {
				state.CooledGroups = append(state.CooledGroups, group)
			}
		}
	}
	return state
}

func listCoolingChannelStates() map[int]*PressureCoolingState {
	result, _ := listCoolingChannelStatesResult()
	return result
}

func listCoolingChannelStatesResult() (map[int]*PressureCoolingState, error) {
	result := make(map[int]*PressureCoolingState)
	if common.RedisEnabled {
		if common.RDB == nil {
			return nil, fmt.Errorf("pressure cooling redis client is nil")
		}
		ctx := context.Background()
		var cursor uint64
		for {
			keys, next, err := common.RDB.Scan(ctx, cursor, "pc:state:*", 200).Result()
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				var chId int
				if _, err := fmt.Sscanf(key, "pc:state:%d", &chId); err != nil || chId <= 0 {
					continue
				}
				st, err := loadPressureCoolingStateRedis(chId)
				if err != nil || st == nil {
					if err != nil {
						if errors.Is(err, errPressureCoolingStateCorrupt) {
							common.SysError(fmt.Sprintf("pressure cooling state corrupt for channel %d: %v", chId, err))
							continue
						}
						return nil, err
					}
					return nil, fmt.Errorf("pressure cooling state is nil for channel %d", chId)
				}
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
				if cp.Scope == "" {
					cp.Scope = "channel"
				}
				cp.CooledGroups = append([]string(nil), st.CooledGroups...)
				result[chId] = &cp
			}
		}
		return true
	})
	return result, nil
}
