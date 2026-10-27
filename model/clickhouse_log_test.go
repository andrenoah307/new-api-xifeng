package model

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickHouseExportStreamPreservesQueryFilters(t *testing.T) {
	// SQLite supports this stream's IFNULL/order expressions, so this regression
	// also runs in CI without an external ClickHouse server.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	require.NoError(t, db.Exec(`CREATE TABLE logs (created_at INTEGER, event_id TEXT, user_id INTEGER, content TEXT)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO logs VALUES
		(100,'b',42,'start'),(200,'a',42,'end'),(150,'c',43,'other-user'),(201,'d',42,'outside')`).Error)
	query := db.Where("logs.user_id = ? AND logs.created_at >= ? AND logs.created_at <= ?", 42, 100, 200)
	var contents []string
	err = streamLogsClickHouse(context.Background(), query, false, 1, func(batch []*Log) error {
		require.Len(t, batch, 1)
		contents = append(contents, batch[0].Content)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"start", "end"}, contents)
}

func TestInitLogDBWithoutDSNReusesRelationalDatabase(t *testing.T) {
	oldDB, oldLogDB := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
	})
	t.Setenv("LOG_SQL_DSN", "")
	t.Setenv("LOG_QUEUE_ENABLED", "false")
	for _, tc := range []struct {
		kind    common.DatabaseType
		dialect gorm.Dialector
	}{
		{common.DatabaseTypeSQLite, sqlite.Open(":memory:")},
		{common.DatabaseTypeMySQL, mysql.New(mysql.Config{DSN: "root@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true})},
		{common.DatabaseTypePostgreSQL, postgres.New(postgres.Config{DSN: "postgres://test@127.0.0.1:1/unused", PreferSimpleProtocol: true})},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			db, err := gorm.Open(tc.dialect, &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			defer conn.Close()
			DB = db
			// Even stale ClickHouse type state must be reset without any log DDL.
			common.SetDatabaseTypes(tc.kind, common.DatabaseTypeClickHouse)
			require.NoError(t, InitLogDB())
			assert.Same(t, DB, LOG_DB)
			assert.Equal(t, tc.kind, common.LogDatabaseType())
			assert.Equal(t, string(tc.kind), LOG_DB.Dialector.Name())
		})
	}
}

func TestIsClickHouseDSN(t *testing.T) {
	cases := []struct {
		dsn  string
		want bool
	}{
		{"clickhouse://default:pass@localhost:9000/logs", true},
		{"tcp://localhost:9000/logs", true},
		{"http://localhost:8123/logs", true},
		{"https://localhost:8443/logs", true},
		{"postgres://root:pass@localhost:5432/db", false},
		{"postgresql://root:pass@localhost:5432/db", false},
		{"root:pass@tcp(localhost:3306)/db", false},
		{"local", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, isClickHouseDSN(c.dsn), "dsn=%q", c.dsn)
	}
}

func TestNormalizeClickHouseDSN(t *testing.T) {
	// https without secure gets secure=true appended
	normalized := normalizeClickHouseDSN("https://default:pass@localhost:8443/logs")
	assert.Contains(t, normalized, "secure=true")
	assert.Contains(t, normalized, "async_insert=1")
	assert.Contains(t, normalized, "wait_for_async_insert=1")
	assert.True(t, strings.HasPrefix(normalized, "https://"))

	// https that already specifies secure keeps the explicit value
	normalized = normalizeClickHouseDSN("https://localhost:8443/logs?secure=false")
	assert.Contains(t, normalized, "secure=false")
	assert.Contains(t, normalized, "async_insert=1")

	// non-https schemes keep their scheme and receive async insert defaults
	normalized = normalizeClickHouseDSN("clickhouse://localhost:9000/logs")
	assert.True(t, strings.HasPrefix(normalized, "clickhouse://"))
	assert.Contains(t, normalized, "async_insert=1")
	assert.Contains(t, normalized, "wait_for_async_insert=1")

	normalized = normalizeClickHouseDSN("tcp://localhost:9000/logs")
	assert.True(t, strings.HasPrefix(normalized, "tcp://"))
	assert.Contains(t, normalized, "async_insert=1")
	assert.Contains(t, normalized, "wait_for_async_insert=1")

	// explicit async insert settings take precedence
	normalized = normalizeClickHouseDSN("clickhouse://localhost:9000/logs?async_insert=0")
	parsed, err := url.Parse(normalized)
	require.NoError(t, err)
	assert.Equal(t, "0", parsed.Query().Get("async_insert"))
	normalized = normalizeClickHouseDSN("clickhouse://localhost:9000/logs?wait_for_async_insert=0")
	assert.Contains(t, normalized, "wait_for_async_insert=0")
}

func TestChooseDBRejectsClickHouseForMainDatabase(t *testing.T) {
	original, had := os.LookupEnv("SQL_DSN")
	t.Cleanup(func() {
		if had {
			require.NoError(t, os.Setenv("SQL_DSN", original))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
	require.NoError(t, os.Setenv("SQL_DSN", "clickhouse://default:pass@localhost:9000/logs"))

	db, dbType, err := chooseDB("SQL_DSN", false)
	require.Error(t, err)
	assert.Nil(t, db)
	assert.Equal(t, common.DatabaseType(""), dbType)
	assert.Contains(t, err.Error(), "does not support ClickHouse")
}

func TestClickHouseLogTTLExpression(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLExpression(0))
	assert.Equal(t, "", clickHouseLogTTLExpression(-5))
	assert.Equal(t, "toDateTime(ifNull(created_at, 0), 'UTC') + INTERVAL 30 DAY DELETE", clickHouseLogTTLExpression(30))
}

func TestClickHouseLogTTLClause(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLClause(0))
	assert.Equal(t, "\nTTL toDateTime(ifNull(created_at, 0), 'UTC') + INTERVAL 7 DAY DELETE", clickHouseLogTTLClause(7))
}

func TestClickHouseLogCreateTableSQL(t *testing.T) {
	withoutTTL := ClickHouseLogCreateTableSQL(0)
	assert.Contains(t, withoutTTL, "CREATE TABLE IF NOT EXISTS logs")
	assert.Contains(t, withoutTTL, "ENGINE = MergeTree()")
	assert.Contains(t, withoutTTL, "PARTITION BY (toYYYYMM(toDateTime(ifNull(created_at, 0), 'UTC')), migration_batch)")
	assert.Contains(t, withoutTTL, "ORDER BY (ifNull(created_at, 0), event_id)")
	assert.NotContains(t, withoutTTL, "TTL ")

	withTTL := ClickHouseLogCreateTableSQL(30)
	assert.Contains(t, withTTL, "ORDER BY (ifNull(created_at, 0), event_id)")
	assert.Contains(t, withTTL, "TTL toDateTime(ifNull(created_at, 0), 'UTC') + INTERVAL 30 DAY DELETE")
}

func TestClickHouseCreateTableHasTTL(t *testing.T) {
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nTTL toDateTime(created_at) + INTERVAL 30 DAY DELETE"))
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...) TTL toDateTime(created_at)"))
	assert.False(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nORDER BY (created_at, request_id)"))
}

func TestClickHouseLogOrder(t *testing.T) {
	assert.Equal(t, "ifNull(created_at, 0) desc, event_id desc", clickHouseLogOrder(""))
	assert.Equal(t, "ifNull(logs.created_at, 0) desc, logs.event_id desc", clickHouseLogOrder("logs."))
}

func TestClickHouseExportOrder(t *testing.T) {
	assert.Equal(t, "ifNull(logs.created_at, 0) asc, logs.event_id asc", clickHouseExportOrder())
}

func TestBuildLogLikeConditionUsesStandardEscape(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(originalLogDatabaseType)
	})
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)

	condition, pattern, err := buildLogLikeCondition("logs.model_name", "gpt_4%")

	require.NoError(t, err)
	assert.Equal(t, "logs.model_name LIKE ? ESCAPE '!'", condition)
	assert.Equal(t, "gpt!_4%", pattern)
}

func TestBuildLogLikeConditionUsesClickHouseEscaping(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(originalLogDatabaseType)
	})
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)

	condition, pattern, err := buildLogLikeCondition("logs.model_name", `gpt_4\mini%`)

	require.NoError(t, err)
	assert.Equal(t, "logs.model_name LIKE ?", condition)
	assert.Equal(t, `gpt\_4\\mini%`, pattern)
}

func TestEnsureLogRequestId(t *testing.T) {
	empty := &Log{}
	ensureLogRequestId(empty)
	assert.NotEmpty(t, empty.RequestId, "empty request id should be backfilled")

	existing := &Log{RequestId: "fixed-request-id"}
	ensureLogRequestId(existing)
	assert.Equal(t, "fixed-request-id", existing.RequestId, "existing request id must be preserved")

	assert.NotPanics(t, func() { ensureLogRequestId(nil) })
}

func TestAssignDisplayLogIds(t *testing.T) {
	logs := []*Log{{}, {}, {}}

	assignDisplayLogIds(logs, 0)
	assert.Equal(t, []int{1, 2, 3}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assignDisplayLogIds(logs, 20)
	assert.Equal(t, []int{21, 22, 23}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assert.NotPanics(t, func() { assignDisplayLogIds(nil, 0) })
}
