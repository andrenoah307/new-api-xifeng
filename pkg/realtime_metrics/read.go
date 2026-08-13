package realtimemetrics

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/go-redis/redis/v8"
)

// seriesMinutes is how much history the dashboard shows. It is bounded by
// seriesRetention, not the other way around.
const seriesMinutes = 60

// readTimeout is looser than the flush path: a dashboard poll can afford to wait
// where a relay request cannot.
const readTimeout = 1500 * time.Millisecond

// ChannelNameFunc resolves a channel id to a display name. The controller passes
// a cache-backed lookup so the read path never touches the database.
type ChannelNameFunc func(channelID int) string

// Read builds the whole dashboard payload. It never returns an error: when Redis
// is unavailable or fails, it degrades to this process's own numbers and marks
// the payload, because a polling dashboard that raises an error toast every ten
// seconds is worse than one that says it is only showing one instance.
func Read(ctx context.Context, channelName ChannelNameFunc) Snapshot {
	nowUnix := time.Now().Unix()
	snapshot := Snapshot{
		RedisEnabled: redisAvailable(),
		NowUnix:      nowUnix,
		Instances:    []InstanceSnapshot{},
		Series:       []MinutePoint{},
		Channels:     []ChannelPoint{},
	}
	if !snapshot.RedisEnabled {
		fillLocal(&snapshot, nowUnix, channelName)
		snapshot.Degraded = true
		snapshot.Warning = "redis_disabled"
		return snapshot
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	nodes, err := common.RDB.ZRangeByScore(readCtx, keyNodes, &redis.ZRangeBy{
		Min: strconv.FormatInt(nowUnix-staleAfterSeconds, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		fillLocal(&snapshot, nowUnix, channelName)
		snapshot.Degraded = true
		snapshot.Warning = "redis_unavailable"
		return snapshot
	}

	pipe := common.RDB.Pipeline()
	instanceCmds := make([]*redis.StringStringMapCmd, 0, len(nodes))
	channelCmds := make([]*redis.StringStringMapCmd, 0, len(nodes))
	for _, node := range nodes {
		instanceCmds = append(instanceCmds, pipe.HGetAll(readCtx, fmt.Sprintf(keyInstanceFmt, node)))
		channelCmds = append(channelCmds, pipe.HGetAll(readCtx, fmt.Sprintf(keyChannelFmt, node)))
	}
	currentMinute := nowUnix - nowUnix%60
	minuteCmds := make([]*redis.StringStringMapCmd, 0, seriesMinutes)
	minuteStamps := make([]int64, 0, seriesMinutes)
	for i := seriesMinutes - 1; i >= 0; i-- {
		stamp := currentMinute - int64(i)*60
		minuteStamps = append(minuteStamps, stamp)
		minuteCmds = append(minuteCmds, pipe.HGetAll(readCtx, fmt.Sprintf(keyMinuteFmt, stamp)))
	}
	channelMinuteCmds := make([]*redis.StringStringMapCmd, 0, channelWindowMins)
	for i := channelWindowMins - 1; i >= 0; i-- {
		channelMinuteCmds = append(channelMinuteCmds, pipe.HGetAll(readCtx, fmt.Sprintf(keyChannelMinFmt, currentMinute-int64(i)*60)))
	}
	if _, err := pipe.Exec(readCtx); err != nil && err != redis.Nil {
		fillLocal(&snapshot, nowUnix, channelName)
		snapshot.Degraded = true
		snapshot.Warning = "redis_unavailable"
		return snapshot
	}

	channelConcurrencySum := map[int]int64{}
	for i, node := range nodes {
		fields, err := instanceCmds[i].Result()
		if err != nil || len(fields) == 0 {
			// The ZSET entry outlived the gauge hash, so the process is gone.
			continue
		}
		lastSeen := parseInt(fields["last_seen_unix"])
		instance := InstanceSnapshot{
			Node:         node,
			LastSeenUnix: lastSeen,
			StaleSeconds: nowUnix - lastSeen,
			InstanceGauges: InstanceGauges{
				ActiveRequests:   parseInt(fields["active_requests"]),
				ActiveBodyBytes:  parseInt(fields["active_body_bytes"]),
				MaxConcurrent:    parseInt(fields["max_concurrent"]),
				MaxBodyBytes:     parseInt(fields["max_body_bytes"]),
				CgroupPermille:   parseInt(fields["cgroup_permille"]),
				CgroupTripped:    parseInt(fields["cgroup_tripped"]) == 1,
				TripCount:        parseInt(fields["trip_count"]),
				ForcedResetCount: parseInt(fields["forced_reset_count"]),
				Goroutines:       parseInt(fields["goroutines"]),
			},
		}
		snapshot.Instances = append(snapshot.Instances, instance)
		addGauges(&snapshot.Totals, instance.InstanceGauges)

		if channelFields, err := channelCmds[i].Result(); err == nil {
			for rawID, rawValue := range channelFields {
				id, convErr := strconv.Atoi(rawID)
				if convErr != nil {
					continue
				}
				channelConcurrencySum[id] += parseInt(rawValue)
			}
		}
	}
	sort.Slice(snapshot.Instances, func(i, j int) bool {
		return snapshot.Instances[i].Node < snapshot.Instances[j].Node
	})

	for i, stamp := range minuteStamps {
		fields, err := minuteCmds[i].Result()
		if err != nil {
			continue
		}
		point := MinutePoint{
			MinuteUnix:       stamp,
			Requests:         parseInt(fields[fieldRequests]),
			Success:          parseInt(fields[fieldSuccess]),
			Errors:           parseInt(fields[fieldErrors]),
			ClientGone:       parseInt(fields[fieldClientGone]),
			PromptTokens:     parseInt(fields[fieldPromptTokens]),
			CompletionTokens: parseInt(fields[fieldCompletionTokens]),
			Quota:            parseInt(fields[fieldQuota]),
			RejGate:          parseInt(fields[RejectionGate]),
			RejConcurrency:   parseInt(fields[RejectionConcurrency]),
			RejBody:          parseInt(fields[RejectionBody]),
			RejMemory:        parseInt(fields[RejectionMemory]),
			RejModelRPM:      parseInt(fields[RejectionModelRPM]),
			RejUserRPM:       parseInt(fields[RejectionUserRPM]),
		}
		// Empty leading minutes are kept: a gap in the chart is information, and
		// dropping them would silently shift the x axis.
		snapshot.Series = append(snapshot.Series, point)
	}

	channelRequests := map[int]int64{}
	channelErrors := map[int]int64{}
	for _, cmd := range channelMinuteCmds {
		fields, err := cmd.Result()
		if err != nil {
			continue
		}
		for rawField, rawValue := range fields {
			id, kind, ok := splitChannelField(rawField)
			if !ok {
				continue
			}
			if kind == "r" {
				channelRequests[id] += parseInt(rawValue)
			} else {
				channelErrors[id] += parseInt(rawValue)
			}
		}
	}
	snapshot.Channels = buildChannels(channelConcurrencySum, channelRequests, channelErrors, channelName)
	return snapshot
}

// fillLocal reports only this process. It is the degraded path, so the series
// stays empty rather than inventing a single point that looks like history.
func fillLocal(snapshot *Snapshot, nowUnix int64, channelName ChannelNameFunc) {
	gauges := LocalGauges()
	name := nodeName
	if name == "" {
		name = common.GetNodeIdentity().Name
	}
	snapshot.Instances = append(snapshot.Instances, InstanceSnapshot{
		Node:           name,
		LastSeenUnix:   nowUnix,
		StaleSeconds:   0,
		InstanceGauges: gauges,
	})
	snapshot.Totals = gauges
	snapshot.Channels = buildChannels(snapshotChannelConcurrency(), nil, nil, channelName)
}

func buildChannels(concurrency map[int]int64, requests map[int]int64, errors map[int]int64, channelName ChannelNameFunc) []ChannelPoint {
	ids := map[int]struct{}{}
	for id := range concurrency {
		ids[id] = struct{}{}
	}
	for id := range requests {
		ids[id] = struct{}{}
	}
	for id := range errors {
		ids[id] = struct{}{}
	}
	points := make([]ChannelPoint, 0, len(ids))
	for id := range ids {
		name := ""
		if channelName != nil {
			name = channelName(id)
		}
		points = append(points, ChannelPoint{
			ChannelID:   id,
			ChannelName: name,
			Concurrency: concurrency[id],
			Requests:    requests[id],
			Errors:      errors[id],
			WindowSecs:  channelWindowMins * 60,
		})
	}
	// A stable default order; the console re-sorts client side.
	sort.Slice(points, func(i, j int) bool {
		if points[i].Concurrency != points[j].Concurrency {
			return points[i].Concurrency > points[j].Concurrency
		}
		return points[i].ChannelID < points[j].ChannelID
	})
	return points
}

func addGauges(total *InstanceGauges, one InstanceGauges) {
	total.ActiveRequests += one.ActiveRequests
	total.ActiveBodyBytes += one.ActiveBodyBytes
	total.MaxConcurrent += one.MaxConcurrent
	total.MaxBodyBytes += one.MaxBodyBytes
	total.TripCount += one.TripCount
	total.ForcedResetCount += one.ForcedResetCount
	total.Goroutines += one.Goroutines
	// Memory pressure does not sum across instances: the worst instance is what
	// decides whether traffic is being shed.
	if one.CgroupPermille > total.CgroupPermille {
		total.CgroupPermille = one.CgroupPermille
	}
	if one.CgroupTripped {
		total.CgroupTripped = true
	}
}

func splitChannelField(field string) (int, string, bool) {
	separator := strings.LastIndex(field, ":")
	if separator <= 0 || separator == len(field)-1 {
		return 0, "", false
	}
	id, err := strconv.Atoi(field[:separator])
	if err != nil {
		return 0, "", false
	}
	kind := field[separator+1:]
	if kind != "r" && kind != "e" {
		return 0, "", false
	}
	return id, kind, true
}

func parseInt(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
