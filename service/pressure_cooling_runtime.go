package service

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

type PressureCoolingRuntime struct {
	Configured     bool   `json:"configured"`
	Enabled        bool   `json:"enabled"`
	Attempts       int64  `json:"attempts"`
	Errors         int64  `json:"errors"`
	TriggerPercent int    `json:"trigger_percent"`
	MinSamples     int    `json:"min_samples"`
	TriggerCount   int    `json:"trigger_count"`
	WindowSeconds  int    `json:"window_seconds"`
	State          string `json:"state"`
	CooldownUntil  int64  `json:"cooldown_until"`
}

type pressureCoolingRuntimeRedisCommands struct {
	channelID int
	cfg       resolvedPressureCoolingConfig
	error     *redis.StringStringMapCmd
	state     *redis.StringStringMapCmd
}

func pressureCoolingRuntimeForConfig(cfg resolvedPressureCoolingConfig) PressureCoolingRuntime {
	return PressureCoolingRuntime{
		Configured:     pressureCoolingErrorConditionConfigured(cfg),
		Enabled:        cfg.Enabled,
		TriggerPercent: cfg.UpstreamErrorTriggerPercent,
		MinSamples:     cfg.UpstreamErrorMinSamples,
		TriggerCount:   cfg.UpstreamErrorTriggerCount,
		WindowSeconds:  normalizePressureCoolingErrorWindow(cfg.ObservationWindowSeconds),
	}
}

// BatchPressureCoolingRuntime reads all requested channel windows and states without
// adding any work to RecordPressureCoolingAttempt's hot path.
func BatchPressureCoolingRuntime(channels []*model.Channel) map[int]PressureCoolingRuntime {
	runtimes := make(map[int]PressureCoolingRuntime, len(channels))
	if len(channels) == 0 {
		return runtimes
	}

	if common.RedisEnabled {
		if common.RDB == nil {
			for _, channel := range channels {
				if channel != nil {
					runtimes[channel.Id] = PressureCoolingRuntime{}
				}
			}
			return runtimes
		}

		now := time.Now().Unix()
		ctx := context.Background()
		pipe := common.RDB.Pipeline()
		commands := make([]pressureCoolingRuntimeRedisCommands, 0, len(channels))
		for _, channel := range channels {
			if channel == nil {
				continue
			}
			cfg := resolvePressureCoolingConfig(channel.GetSetting().PressureCooling)
			commands = append(commands, pressureCoolingRuntimeRedisCommands{
				channelID: channel.Id,
				cfg:       cfg,
				error:     pipe.HGetAll(ctx, pressureCoolingErrorWindowKey(channel.Id, cfg.ObservationWindowSeconds, now)),
				state:     pipe.HGetAll(ctx, pressureCoolingRedisKey(channel.Id)),
			})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			for _, command := range commands {
				runtimes[command.channelID] = PressureCoolingRuntime{}
			}
			return runtimes
		}

		for _, command := range commands {
			runtime := pressureCoolingRuntimeForConfig(command.cfg)
			values := command.error.Val()
			runtime.Attempts, _ = strconv.ParseInt(values["a"], 10, 64)
			runtime.Errors, _ = strconv.ParseInt(values["e"], 10, 64)
			if state, err := parsePressureCoolingStateFields(command.channelID, command.state.Val()); err == nil && state != nil {
				runtime.State = state.State
				runtime.CooldownUntil = state.CooldownUntil
			}
			runtimes[command.channelID] = runtime
		}
		return runtimes
	}

	now := time.Now().Unix()
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		cfg := resolvePressureCoolingConfig(channel.GetSetting().PressureCooling)
		runtime := pressureCoolingRuntimeForConfig(cfg)
		runtime.Attempts, runtime.Errors = loadPressureCoolingErrorWindowAt(channel.Id, cfg.ObservationWindowSeconds, now)
		state := loadPressureCoolingStateMemory(channel.Id)
		runtime.State = state.State
		runtime.CooldownUntil = state.CooldownUntil
		runtimes[channel.Id] = runtime
	}
	return runtimes
}
