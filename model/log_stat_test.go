package model

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSumUsedQuotaFiltersRequestIdentifiers(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })

	now := time.Now().Unix()
	logs := []*Log{
		{UserId: 42, CreatedAt: now, Type: LogTypeConsume, Quota: 10, PromptTokens: 2, CompletionTokens: 3, RequestId: "req-a", UpstreamRequestId: "up-a"},
		{UserId: 42, CreatedAt: now, Type: LogTypeConsume, Quota: 20, PromptTokens: 4, CompletionTokens: 5, RequestId: "req-b", UpstreamRequestId: "up-a"},
		{UserId: 42, CreatedAt: now, Type: LogTypeConsume, Quota: 30, PromptTokens: 6, CompletionTokens: 7, RequestId: "req-a", UpstreamRequestId: "up-b"},
		{UserId: 42, CreatedAt: now, Type: LogTypeManage, Quota: 100, PromptTokens: 100, CompletionTokens: 100, RequestId: "req-a", UpstreamRequestId: "up-a"},
	}
	require.NoError(t, DB.Create(&logs).Error)

	tests := []struct {
		name       string
		userID     int
		requestID  string
		upstreamID string
		wantQuota  int
		wantRPM    int
		wantTPM    int
	}{
		{name: "request id", requestID: "req-a", wantQuota: 40, wantRPM: 2, wantTPM: 18},
		{name: "upstream request id", upstreamID: "up-a", wantQuota: 30, wantRPM: 2, wantTPM: 14},
		{name: "both identifiers are conjunctive", requestID: "req-a", upstreamID: "up-a", wantQuota: 10, wantRPM: 1, wantTPM: 5},
		{name: "user query also applies identifiers", userID: 42, requestID: "req-b", wantQuota: 20, wantRPM: 1, wantTPM: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stat, err := SumUsedQuota(
				context.Background(),
				tt.userID,
				LogTypeConsume,
				now-10,
				now+10,
				"",
				"",
				"",
				0,
				"",
				tt.requestID,
				tt.upstreamID,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.wantQuota, stat.Quota)
			assert.Equal(t, tt.wantRPM, stat.Rpm)
			assert.Equal(t, tt.wantTPM, stat.Tpm)
		})
	}
}

func TestSumUsedQuotaAppliesAllFiltersTogether(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })

	now := time.Now().Unix()
	rows := []*Log{
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Username: "alice", TokenName: "production", ModelName: "gpt-5", ChannelId: 9, Group: "default", RequestId: "req-all", UpstreamRequestId: "up-all", Quota: 13, PromptTokens: 2, CompletionTokens: 3},
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Username: "bob", TokenName: "production", ModelName: "gpt-5", ChannelId: 9, Group: "default", RequestId: "req-all", UpstreamRequestId: "up-all", Quota: 17},
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Username: "alice", TokenName: "other", ModelName: "gpt-5", ChannelId: 9, Group: "default", RequestId: "req-all", UpstreamRequestId: "up-all", Quota: 19},
	}
	require.NoError(t, DB.Create(&rows).Error)

	stat, err := SumUsedQuota(
		context.Background(),
		0,
		LogTypeConsume,
		now-10,
		now+10,
		"gpt-5",
		"alice",
		"production",
		9,
		"default",
		"req-all",
		"up-all",
	)
	require.NoError(t, err)
	assert.Equal(t, 13, stat.Quota)
}

func TestSumUsedQuotaReturnsErrorWhenLogDatabaseCannotBeQueried(t *testing.T) {
	oldLogDB := LOG_DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = brokenDB
	t.Cleanup(func() { LOG_DB = oldLogDB })

	_, err = SumUsedQuota(context.Background(), 0, LogTypeConsume, 0, 0, "", "", "", 0, "", "", "")
	require.Error(t, err)
	assert.Equal(t, "查询统计数据失败", err.Error())
}

func TestSumUsedQuotaRejectsInvalidLikeFilters(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		modelName string
		username  string
	}{
		{name: "invalid username", username: "%"},
		{name: "invalid model", modelName: "%"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := SumUsedQuota(
				context.Background(),
				0,
				LogTypeConsume,
				0,
				0,
				testCase.modelName,
				testCase.username,
				"",
				0,
				"",
				"",
				"",
			)
			require.Error(t, err)
		})
	}
}

func TestSumUsedQuotaReportsRpmQueryErrors(t *testing.T) {
	oldLogDB := LOG_DB
	partialDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, partialDB.Exec("CREATE TABLE logs (quota INTEGER, type INTEGER, created_at INTEGER)").Error)
	LOG_DB = partialDB
	t.Cleanup(func() { LOG_DB = oldLogDB })

	_, err = SumUsedQuota(context.Background(), 0, LogTypeConsume, 0, 0, "", "", "", 0, "", "", "")
	require.Error(t, err)
	assert.Equal(t, "查询统计数据失败", err.Error())
}
