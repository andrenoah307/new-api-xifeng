package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogCompositeIndexDefinitions(t *testing.T) {
	statement := &gorm.Statement{DB: DB}
	require.NoError(t, statement.Parse(&Log{}))
	indexes := statement.Schema.ParseIndexes()

	want := map[string][]string{
		"idx_logs_model_name_created_at": {"model_name", "created_at", "id", "type"},
		"idx_logs_username_created_at":   {"username", "created_at", "id", "type"},
		"idx_logs_token_name_created_at": {"token_name", "created_at", "id", "type"},
		"idx_logs_type_created_at_quota": {"type", "created_at", "id", "quota"},
	}
	for name, wantColumns := range want {
		index, ok := indexes[name]
		require.Truef(t, ok, "index %s must be declared", name)
		gotColumns := make([]string, len(index.Fields))
		for i, field := range index.Fields {
			gotColumns[i] = field.DBName
		}
		assert.Equal(t, wantColumns, gotColumns, "index %s columns", name)
	}

	for _, removed := range []string{
		"idx_logs_username",
		"idx_logs_token_name",
		"idx_logs_model_name",
		"idx_logs_channel_id",
		"idx_logs_type_created_at",
	} {
		assert.NotContains(t, indexes, removed, "obsolete index tag %s must stay removed", removed)
	}

	assert.Contains(t, indexes, "index_username_model_name")
	assert.Contains(t, indexes, "idx_created_at_type")
	assert.Contains(t, indexes, "idx_logs_request_id")
	assert.Contains(t, indexes, "idx_logs_upstream_request_id")
}

func TestLogListOrderSelection(t *testing.T) {
	tests := []struct {
		name            string
		usingClickHouse bool
		filters         logOrderFilters
		want            string
	}{
		{name: "no filters", want: "logs.id desc"},
		{name: "LIKE model", filters: logOrderFilters{modelName: "%gpt%"}, want: "logs.id desc"},
		{name: "LIKE username", filters: logOrderFilters{username: "%alice%"}, want: "logs.id desc"},
		{name: "exact model", filters: logOrderFilters{modelName: "gpt-5"}, want: "logs.created_at desc, logs.id desc"},
		{name: "exact username", filters: logOrderFilters{username: "alice"}, want: "logs.created_at desc, logs.id desc"},
		{name: "token name", filters: logOrderFilters{tokenName: "production"}, want: "logs.created_at desc, logs.id desc"},
		{name: "group", filters: logOrderFilters{group: "default"}, want: "logs.created_at desc, logs.id desc"},
		{name: "channel", filters: logOrderFilters{channel: 7}, want: "logs.created_at desc, logs.id desc"},
		{name: "request id", filters: logOrderFilters{requestId: "req-1"}, want: "logs.id desc"},
		{name: "request id short-circuits exact model", filters: logOrderFilters{requestId: "req-1", modelName: "gpt-5"}, want: "logs.id desc"},
		{name: "upstream request id", filters: logOrderFilters{upstreamRequestId: "up-1"}, want: "logs.id desc"},
		{name: "clickhouse keeps native order", usingClickHouse: true, filters: logOrderFilters{group: "default", requestId: "req-1"}, want: "logs.created_at desc, logs.request_id desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getAllLogsOrder(tt.filters, tt.usingClickHouse))
		})
	}
}

func TestUserLogOrderUsesUserTimeIndex(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() { common.SetLogDatabaseType(originalLogDatabaseType) })
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })

	rows := []*Log{
		{Id: 20002, UserId: 24, CreatedAt: 300, Type: LogTypeConsume, RequestId: "req-a"},
		{Id: 20001, UserId: 24, CreatedAt: 300, Type: LogTypeConsume, RequestId: "req-z"},
		{Id: 20003, UserId: 24, CreatedAt: 100, Type: LogTypeConsume, RequestId: "req-old"},
	}
	require.NoError(t, DB.Create(&rows).Error)

	tests := []struct {
		name         string
		databaseType common.DatabaseType
		want         []string
	}{
		{name: "relational database", databaseType: common.DatabaseTypeSQLite, want: []string{"req-a", "req-z", "req-old"}},
		{name: "ClickHouse", databaseType: common.DatabaseTypeClickHouse, want: []string{"req-z", "req-a", "req-old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common.SetLogDatabaseType(tt.databaseType)
			logs, _, err := GetUserLogs(24, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", int64(len(rows)))
			require.NoError(t, err)
			requestIds := make([]string, len(logs))
			for i := range logs {
				requestIds[i] = logs[i].RequestId
			}
			assert.Equal(t, tt.want, requestIds)
		})
	}
}

func TestLogTextFilterEqualityPrefixSharesLikeDecision(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() { common.SetLogDatabaseType(originalLogDatabaseType) })
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)

	tests := []struct {
		name       string
		value      string
		wantEquals bool
		wantLike   bool
	}{
		{name: "empty", value: "", wantEquals: false, wantLike: false},
		{name: "exact", value: "gpt-5", wantEquals: true, wantLike: false},
		{name: "percent suffix", value: "gpt%", wantEquals: false, wantLike: true},
		{name: "percent prefix", value: "%gpt", wantEquals: false, wantLike: true},
		{name: "percent in middle", value: "g%pt", wantEquals: false, wantLike: true},
		{name: "underscore only", value: "gpt_5", wantEquals: true, wantLike: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEquals, logTextFilterProvidesEqualityPrefix(tt.value))

			filtered, err := applyExplicitLogTextFilter(
				DB.Session(&gorm.Session{DryRun: true}),
				"logs.model_name",
				tt.value,
			)
			require.NoError(t, err)
			statement := filtered.Find(&Log{}).Statement
			sql := strings.ToUpper(statement.SQL.String())
			assert.Equal(t, tt.wantLike, strings.Contains(sql, " LIKE "))
			if tt.wantEquals {
				assert.Contains(t, sql, "LOGS.MODEL_NAME = ")
			}
		})
	}

	_, err := applyExplicitLogTextFilter(
		DB.Session(&gorm.Session{DryRun: true}),
		"logs.model_name",
		"%",
	)
	require.Error(t, err)
}

func TestGetAllLogsOrderingMatrixUsesAppliedFilters(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })

	rows := []*Log{
		{Id: 21002, CreatedAt: 100, Type: LogTypeConsume, ModelName: "gpt-5", Username: "alice", TokenName: "production", Group: "default", RequestId: "req-a", UpstreamRequestId: "up-a"},
		{Id: 21001, CreatedAt: 300, Type: LogTypeConsume, ModelName: "gpt-5", Username: "alice", TokenName: "production", Group: "default", RequestId: "req-a", UpstreamRequestId: "up-a"},
		{Id: 21003, CreatedAt: 200, Type: LogTypeConsume, ModelName: "other", Username: "bob", TokenName: "other", Group: "other", RequestId: "req-b", UpstreamRequestId: "up-b"},
	}
	require.NoError(t, DB.Create(&rows).Error)

	tests := []struct {
		name       string
		logType    int
		modelName  string
		username   string
		tokenName  string
		group      string
		requestId  string
		upstreamId string
		wantIds    []int
	}{
		{name: "no filters", wantIds: []int{21003, 21002, 21001}},
		{name: "only type", logType: LogTypeConsume, wantIds: []int{21003, 21002, 21001}},
		{name: "LIKE model", modelName: "%gpt%", wantIds: []int{21002, 21001}},
		{name: "LIKE username", username: "%ali%", wantIds: []int{21002, 21001}},
		{name: "exact model", modelName: "gpt-5", wantIds: []int{21001, 21002}},
		{name: "exact username", username: "alice", wantIds: []int{21001, 21002}},
		{name: "token name", tokenName: "production", wantIds: []int{21001, 21002}},
		{name: "group", group: "default", wantIds: []int{21001, 21002}},
		{name: "request id", requestId: "req-a", wantIds: []int{21002, 21001}},
		{name: "request id plus exact model", requestId: "req-a", modelName: "gpt-5", wantIds: []int{21002, 21001}},
		{name: "upstream request id", upstreamId: "up-a", wantIds: []int{21002, 21001}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := GetAllLogs(
				tt.logType,
				0,
				0,
				tt.modelName,
				tt.username,
				tt.tokenName,
				0,
				10,
				0,
				tt.group,
				tt.requestId,
				tt.upstreamId,
				0,
			)
			require.NoError(t, err)
			gotIds := make([]int, len(got))
			for i := range got {
				gotIds[i] = got[i].Id
			}
			assert.Equal(t, tt.wantIds, gotIds)
		})
	}
}

func TestGetAllLogsUsesCachedTotal(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })
	require.NoError(t, DB.Create(&Log{Id: 22001, CreatedAt: 100, Type: LogTypeConsume}).Error)

	logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", 77)
	require.NoError(t, err)
	assert.Equal(t, int64(77), total)
	require.Len(t, logs, 1)
}

func TestGetAllLogsReportsCountAndFindErrors(t *testing.T) {
	oldLogDB := LOG_DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = brokenDB
	t.Cleanup(func() { LOG_DB = oldLogDB })

	_, _, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", 0)
	require.Error(t, err)
	_, _, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", 1)
	require.Error(t, err)
}

func TestGetUserLogsCachedTotalAndCountCap(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM logs").Error })
	require.NoError(t, DB.Create(&Log{UserId: 23, CreatedAt: 100, Type: LogTypeConsume}).Error)

	logs, total, err := GetUserLogs(23, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 42)
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	require.Len(t, logs, 1)

	oldLimit := common.LogSearchCountLimit
	common.LogSearchCountLimit = 0
	t.Cleanup(func() { common.LogSearchCountLimit = oldLimit })
	_, total, err = GetUserLogs(23, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
}

func TestGetUserLogsReportsCountAndFindErrors(t *testing.T) {
	oldLogDB := LOG_DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = brokenDB
	t.Cleanup(func() { LOG_DB = oldLogDB })

	_, _, err = GetUserLogs(23, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 0)
	require.Error(t, err)
	_, _, err = GetUserLogs(23, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 1)
	require.Error(t, err)
}
