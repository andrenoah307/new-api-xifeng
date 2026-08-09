package model

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type tokenCacheRedis struct {
	mu       sync.Mutex
	hashes   map[string]map[string]string
	commands map[string]int
	events   chan string
}

func setupTokenCacheRedis(t *testing.T) *tokenCacheRedis {
	t.Helper()
	fake := &tokenCacheRedis{
		hashes:   make(map[string]map[string]string),
		commands: make(map[string]int),
		events:   make(chan string, 64),
	}
	client := redis.NewClient(&redis.Options{
		Addr:       "token-cache-test",
		MaxRetries: -1,
		PoolSize:   1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go fake.serve(serverConn)
			return clientConn, nil
		},
	})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = client
	common.RedisEnabled = true
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		common.RDB = oldClient
		common.RedisEnabled = oldEnabled
	})
	return fake
}

func (f *tokenCacheRedis) setTokenHash(key string, values map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copyValues := make(map[string]string, len(values))
	for field, value := range values {
		copyValues[field] = value
	}
	f.hashes[fmt.Sprintf("token:%s", common.GenerateHMAC(key))] = copyValues
}

func (f *tokenCacheRedis) tokenHash(key string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := f.hashes[fmt.Sprintf("token:%s", common.GenerateHMAC(key))]
	copyValues := make(map[string]string, len(values))
	for field, value := range values {
		copyValues[field] = value
	}
	return copyValues
}

func (f *tokenCacheRedis) commandCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commands[strings.ToUpper(name)]
}

func (f *tokenCacheRedis) waitForCommand(t *testing.T, name string) {
	t.Helper()
	wanted := strings.ToUpper(name)
	if f.commandCount(wanted) > 0 {
		return
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case command := <-f.events:
			if command == wanted {
				return
			}
		case <-timer.C:
			require.FailNow(t, "Redis command was not observed", wanted)
		}
	}
}

func (f *tokenCacheRedis) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var queued [][]string
	inTransaction := false
	for {
		command, err := readTokenCacheRedisCommand(reader)
		if err != nil {
			return
		}
		name := strings.ToUpper(command[0])
		var response string
		switch name {
		case "MULTI":
			queued = nil
			inTransaction = true
			response = "+OK\r\n"
		case "EXEC":
			var result strings.Builder
			fmt.Fprintf(&result, "*%d\r\n", len(queued))
			for _, queuedCommand := range queued {
				result.WriteString(f.apply(queuedCommand))
			}
			queued = nil
			inTransaction = false
			response = result.String()
		default:
			if inTransaction {
				queued = append(queued, command)
				response = "+QUEUED\r\n"
			} else {
				response = f.apply(command)
			}
		}
		if _, err = io.WriteString(conn, response); err != nil {
			return
		}
		f.mu.Lock()
		f.commands[name]++
		f.mu.Unlock()
		f.events <- name
	}
}

func (f *tokenCacheRedis) apply(command []string) string {
	name := strings.ToUpper(command[0])
	switch name {
	case "HGETALL":
		f.mu.Lock()
		values := f.hashes[command[1]]
		fields := make([]string, 0, len(values))
		for field := range values {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		var response strings.Builder
		fmt.Fprintf(&response, "*%d\r\n", len(fields)*2)
		for _, field := range fields {
			response.WriteString(tokenCacheRedisBulk(field))
			response.WriteString(tokenCacheRedisBulk(values[field]))
		}
		f.mu.Unlock()
		return response.String()
	case "HSET":
		f.mu.Lock()
		values := f.hashes[command[1]]
		if values == nil {
			values = make(map[string]string)
			f.hashes[command[1]] = values
		}
		added := 0
		for i := 2; i+1 < len(command); i += 2 {
			if _, exists := values[command[i]]; !exists {
				added++
			}
			values[command[i]] = command[i+1]
		}
		f.mu.Unlock()
		return fmt.Sprintf(":%d\r\n", added)
	case "HINCRBY":
		f.mu.Lock()
		values := f.hashes[command[1]]
		if values == nil {
			values = make(map[string]string)
			f.hashes[command[1]] = values
		}
		current, _ := strconv.ParseInt(values[command[2]], 10, 64)
		delta, _ := strconv.ParseInt(command[3], 10, 64)
		current += delta
		values[command[2]] = strconv.FormatInt(current, 10)
		f.mu.Unlock()
		return fmt.Sprintf(":%d\r\n", current)
	case "DEL":
		f.mu.Lock()
		deleted := 0
		for _, key := range command[1:] {
			if _, exists := f.hashes[key]; exists {
				delete(f.hashes, key)
				deleted++
			}
		}
		f.mu.Unlock()
		return fmt.Sprintf(":%d\r\n", deleted)
	case "EXPIRE", "PEXPIRE":
		return ":1\r\n"
	case "TTL":
		return ":60\r\n"
	default:
		return "-ERR unsupported command\r\n"
	}
}

func readTokenCacheRedisCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
	if err != nil {
		return nil, err
	}
	command := make([]string, count)
	for i := range command {
		bulkHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		length, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(bulkHeader, "$")))
		if parseErr != nil {
			return nil, parseErr
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return nil, readErr
		}
		command[i] = string(payload[:length])
	}
	return command, nil
}

func tokenCacheRedisBulk(value string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
}

func TestGetTokenByKeyHitsCurrentSentinelProjection(t *testing.T) {
	fake := setupTokenCacheRedis(t)
	const key = "sentinel-cache-hit"
	fake.setTokenHash(key, map[string]string{
		"Id":                   "42",
		"Status":               strconv.Itoa(common.TokenStatusEnabled),
		"Name":                 "cached metadata",
		"RemainQuota":          "987",
		"PeriodType":           common.TokenPeriodTypeDays,
		"PeriodDays":           "3",
		"PeriodQuotaLimit":     "456",
		"PeriodLimitUnit":      "quota",
		"PeriodAnchorAt":       "123",
		"PeriodStartAt":        "-1",
		"PeriodUsedQuota":      "-1",
		"PeriodResetAt":        "0",
		"PeriodRemainingQuota": "0",
	})

	oldDB := DB
	DB = nil
	t.Cleanup(func() { DB = oldDB })
	token, err := GetTokenByKey(key, false)
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, key, token.Key)
	assert.Equal(t, "cached metadata", token.Name)
	assert.Equal(t, 987, token.RemainQuota)
	assert.Equal(t, common.TokenPeriodTypeDays, token.PeriodType)
	assert.Equal(t, 3, token.PeriodDays)
	assert.Equal(t, int64(456), token.PeriodQuotaLimit)
	assert.Equal(t, "quota", token.PeriodLimitUnit)
	assert.Equal(t, int64(123), token.PeriodAnchorAt)
	assert.Equal(t, int64(-1), token.PeriodStartAt)
	assert.Equal(t, int64(-1), token.PeriodUsedQuota)
	assert.Zero(t, token.PeriodResetAt)
	assert.Zero(t, token.PeriodRemainingQuota)
	fake.waitForCommand(t, "HGETALL")
	assert.Equal(t, 1, fake.commandCount("HGETALL"))
	assert.Zero(t, fake.commandCount("HSET"), "a cache hit must not rewrite the hash")
}

func TestCacheGetTokenByKeyRejectsLegacyPeriodMarkers(t *testing.T) {
	fake := setupTokenCacheRedis(t)
	const key = "legacy-cache-shape"
	tests := []struct {
		name    string
		markers map[string]string
	}{
		{name: "missing fields", markers: map[string]string{}},
		{name: "zero values", markers: map[string]string{"PeriodStartAt": "0", "PeriodUsedQuota": "0"}},
		{name: "start marker only", markers: map[string]string{"PeriodStartAt": "-1"}},
		{name: "used marker only", markers: map[string]string{"PeriodUsedQuota": "-1"}},
		{name: "other negative value", markers: map[string]string{"PeriodStartAt": "-2", "PeriodUsedQuota": "-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{"Id": "1", "Name": "stale"}
			for field, value := range tt.markers {
				values[field] = value
			}
			fake.setTokenHash(key, values)
			_, err := cacheGetTokenByKey(key)
			assert.Error(t, err)
		})
	}
}

func TestGetTokenByKeyRebuildsLegacyHashFromDatabase(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	fake := setupTokenCacheRedis(t)
	token := seedPeriodToken(t, db, "", 0, 0, 0)
	token.Name = "database metadata"
	token.PeriodQuotaLimit = 0
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"name":               token.Name,
		"period_quota_limit": 0,
	}).Error)
	fake.setTokenHash(token.Key, map[string]string{"Id": strconv.Itoa(token.Id), "Name": "stale metadata"})

	loaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, "database metadata", loaded.Name)
	fake.waitForCommand(t, "EXEC")
	rebuilt := fake.tokenHash(token.Key)
	assert.Equal(t, "-1", rebuilt["PeriodStartAt"])
	assert.Equal(t, "-1", rebuilt["PeriodUsedQuota"])
	assert.Equal(t, "database metadata", rebuilt["Name"])
}

func TestGetTokenByIdReadsPrimaryWithoutTouchingTokenCache(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	fake := setupTokenCacheRedis(t)
	token := seedPeriodToken(t, db, "", 0, 0, 0)
	token.Name = "primary row"
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("name", token.Name).Error)

	loaded, err := GetTokenById(token.Id)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, token.Id, loaded.Id)
	assert.Equal(t, token.Key, loaded.Key)
	assert.Equal(t, "primary row", loaded.Name)
	assert.Zero(t, fake.commandCount("DEL"), "GetTokenById is a read and must not invalidate the token hash")
	assert.Zero(t, fake.commandCount("HSET"), "GetTokenById is a read and must not rewrite the token hash")
}

func TestAdjustTokenQuotaUpdatesRedisOnlyAfterDatabaseSuccess(t *testing.T) {
	db := setupTokenPeriodTestDB(t)
	fake := setupTokenCacheRedis(t)
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
	hint := &TokenPeriodAdjustmentHint{KnownDisabled: true}

	deleted := seedPeriodToken(t, db, "", 0, 0, 0)
	deleted.PeriodQuotaLimit = 0
	require.NoError(t, db.Model(&Token{}).Where("id = ?", deleted.Id).Update("period_quota_limit", 0).Error)
	fake.setTokenHash(deleted.Key, map[string]string{
		"RemainQuota":     "100",
		"PeriodStartAt":   "-1",
		"PeriodUsedQuota": "-1",
	})
	require.NoError(t, db.Delete(&Token{}, deleted.Id).Error)
	assert.ErrorIs(t, AdjustTokenQuota(deleted.Id, deleted.Key, 10, 0, hint), gorm.ErrRecordNotFound)
	assert.Equal(t, "100", fake.tokenHash(deleted.Key)["RemainQuota"])
	assert.Zero(t, fake.commandCount("HINCRBY"), "a failed database adjustment must not enqueue a Redis delta")

	live := seedPeriodToken(t, db, "", 1, 0, 0)
	live.PeriodQuotaLimit = 0
	require.NoError(t, db.Model(&Token{}).Where("id = ?", live.Id).Update("period_quota_limit", 0).Error)
	fake.setTokenHash(live.Key, map[string]string{
		"RemainQuota":     "100",
		"PeriodStartAt":   "-1",
		"PeriodUsedQuota": "-1",
	})
	require.NoError(t, AdjustTokenQuota(live.Id, live.Key, 10, 0, hint))
	fake.waitForCommand(t, "EXEC")
	assert.Equal(t, 1, fake.commandCount("HINCRBY"))
	assert.Equal(t, "90", fake.tokenHash(live.Key)["RemainQuota"])
}
