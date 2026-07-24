package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, normalized, "wait_for_async_insert=0")
	assert.True(t, strings.HasPrefix(normalized, "https://"))

	// https that already specifies secure keeps the explicit value
	normalized = normalizeClickHouseDSN("https://localhost:8443/logs?secure=false")
	assert.Contains(t, normalized, "secure=false")
	assert.Contains(t, normalized, "async_insert=1")

	// non-https schemes keep their scheme and receive async insert defaults
	normalized = normalizeClickHouseDSN("clickhouse://localhost:9000/logs")
	assert.True(t, strings.HasPrefix(normalized, "clickhouse://"))
	assert.Contains(t, normalized, "async_insert=1")
	assert.Contains(t, normalized, "wait_for_async_insert=0")

	normalized = normalizeClickHouseDSN("tcp://localhost:9000/logs")
	assert.True(t, strings.HasPrefix(normalized, "tcp://"))
	assert.Contains(t, normalized, "async_insert=1")
	assert.Contains(t, normalized, "wait_for_async_insert=0")

	// explicit async insert settings take precedence
	normalized = normalizeClickHouseDSN("clickhouse://localhost:9000/logs?async_insert=0")
	assert.Contains(t, normalized, "async_insert=0")
	assert.NotContains(t, normalized, "async_insert=1")
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
	assert.Equal(t, "toDateTime(created_at) + INTERVAL 30 DAY DELETE", clickHouseLogTTLExpression(30))
}

func TestClickHouseLogTTLClause(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLClause(0))
	assert.Equal(t, "\nTTL toDateTime(created_at) + INTERVAL 7 DAY DELETE", clickHouseLogTTLClause(7))
}

func TestClickHouseLogCreateTableSQL(t *testing.T) {
	withoutTTL := clickHouseLogCreateTableSQL(0)
	assert.Contains(t, withoutTTL, "CREATE TABLE IF NOT EXISTS logs")
	assert.Contains(t, withoutTTL, "ENGINE = MergeTree()")
	assert.Contains(t, withoutTTL, "PARTITION BY toYYYYMM(toDateTime(created_at))")
	assert.Contains(t, withoutTTL, "ORDER BY (created_at, request_id)")
	assert.NotContains(t, withoutTTL, "TTL ")

	withTTL := clickHouseLogCreateTableSQL(30)
	assert.Contains(t, withTTL, "ORDER BY (created_at, request_id)")
	assert.Contains(t, withTTL, "TTL toDateTime(created_at) + INTERVAL 30 DAY DELETE")
}

func TestClickHouseCreateTableHasTTL(t *testing.T) {
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nTTL toDateTime(created_at) + INTERVAL 30 DAY DELETE"))
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...) TTL toDateTime(created_at)"))
	assert.False(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nORDER BY (created_at, request_id)"))
}

func TestClickHouseLogOrder(t *testing.T) {
	assert.Equal(t, "created_at desc, request_id desc", clickHouseLogOrder(""))
	assert.Equal(t, "logs.created_at desc, logs.request_id desc", clickHouseLogOrder("logs."))
}

func TestClickHouseExportOrder(t *testing.T) {
	assert.Equal(t, "logs.created_at asc, logs.request_id asc", clickHouseExportOrder())
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
