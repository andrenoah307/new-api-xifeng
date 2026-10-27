package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func setupClickHouseLogTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("TEST_CLICKHOUSE_DSN is required for isolated ClickHouse integration tests")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	admin, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	name := "log_test_" + uuid.New().String()[:8]
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	u.Path = "/" + name
	db, err := gorm.Open(clickhouse.Open(normalizeClickHouseDSN(u.String())), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldType := LOG_DB, common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	initCol()
	t.Cleanup(func() {
		LOG_DB = oldDB
		common.SetLogDatabaseType(oldType)
		initCol()
		assert.NoError(t, admin.Exec("DROP DATABASE "+name+" SYNC").Error)
		for _, conn := range []*gorm.DB{db, admin} {
			sqlDB, err := conn.DB()
			if assert.NoError(t, err) {
				assert.NoError(t, sqlDB.Close())
			}
		}
	})
	t.Setenv("LOG_SQL_CLICKHOUSE_TTL_DAYS", "0")
	require.NoError(t, migrateClickHouseLogDB())
	return db
}

func TestClickHouseLogHistoricalValues(t *testing.T) {
	db := setupClickHouseLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs (id, created_at, quota, prompt_tokens, channel_name, session_id, request_id)
		VALUES (9007199254740993, 1700000000, 4294967296, 8589934592, 'historical channel', 'session', NULL)`).Error)
	var row struct {
		ID           int64
		Quota        int64
		PromptTokens int64
		ChannelName  string
		SessionID    string
		RequestID    *string
	}
	require.NoError(t, db.Table("logs").Take(&row).Error)
	assert.Equal(t, int64(9007199254740993), row.ID)
	assert.Equal(t, int64(4294967296), row.Quota)
	assert.Equal(t, int64(8589934592), row.PromptTokens)
	assert.Equal(t, "historical channel", row.ChannelName)
	assert.Equal(t, "session", row.SessionID)
	assert.Nil(t, row.RequestID)
	var ddl string
	require.NoError(t, db.Raw("SHOW CREATE TABLE logs").Scan(&ddl).Error)
	assert.False(t, clickHouseCreateTableHasTTL(ddl))
	require.NoError(t, db.Exec("INSERT INTO logs (id, created_at) VALUES (2, NULL)").Error)
	// TTL must also be valid for a schema that preserves nullable timestamps.
	require.NoError(t, syncClickHouseLogTTL(36500))
	require.NoError(t, syncClickHouseLogTTL(0))
}

func TestClickHouseLogExportsApplyFiltersAcrossBatches(t *testing.T) {
	db := setupClickHouseLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs
		(user_id,username,created_at,type,model_name,token_name,"group",request_id,upstream_request_id,event_id,content)
		SELECT 42,'owner',1700000000 + number % 2,2,'export-model','export-token','export-group','shared-request','upstream',
		concat('event-',leftPad(toString(number),4,'0')),toString(number) FROM numbers(1001)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,username,created_at,type,model_name,token_name,"group",request_id,upstream_request_id,event_id) VALUES
		(43,'other',1700000000,2,'export-model','export-token','export-group','shared-request','upstream','other-user'),
		(42,'owner',1699999999,2,'export-model','export-token','export-group','shared-request','upstream','before'),
		(42,'owner',1700000002,2,'export-model','export-token','export-group','shared-request','upstream','after'),
		(42,'owner',NULL,2,'export-model','export-token','export-group','shared-request','upstream','null-time'),
		(42,'owner',1700000000,5,'export-model','export-token','export-group','shared-request','upstream','wrong-type'),
		(42,'owner',1700000000,2,'other-model','export-token','export-group','shared-request','upstream','wrong-model')`).Error)
	for _, scope := range []string{"admin", "user"} {
		t.Run(scope, func(t *testing.T) {
			var exported []*Log
			var sizes []int
			collect := func(batch []*Log) error {
				sizes = append(sizes, len(batch))
				exported = append(exported, batch...)
				return nil
			}
			var err error
			if scope == "admin" {
				err = ExportAllLogs(context.Background(), LogTypeConsume, 1700000000, 1700000001, "export-model", "owner", "export-token", 0, "export-group", "shared-request", "upstream", collect)
			} else {
				err = ExportUserLogs(context.Background(), 42, LogTypeConsume, 1700000000, 1700000001, "export-model", "export-token", "export-group", "shared-request", "upstream", collect)
			}
			require.NoError(t, err)
			require.Len(t, exported, 1001)
			assert.Equal(t, []int{1000, 1}, sizes)
			seen := make(map[string]bool)
			for _, row := range exported {
				assert.Equal(t, 42, row.UserId)
				assert.False(t, seen[row.Content], "each event must be exported once")
				seen[row.Content] = true
			}
			assert.Equal(t, int64(1700000000), exported[0].CreatedAt)
			assert.Equal(t, int64(1700000001), exported[len(exported)-1].CreatedAt)
		})
	}
}

func TestClickHouseLogWriteVisibleOnAcknowledgment(t *testing.T) {
	db := setupClickHouseLogTest(t)
	row := &Log{UserId: 4, CreatedAt: time.Now().Unix(), Type: LogTypeConsume, RequestId: "same-request", Quota: 4294967296}
	require.NoError(t, createLog(row))
	var count int64
	require.NoError(t, db.WithContext(context.Background()).Model(&Log{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestClickHouseLogImportPublicationAndRetry(t *testing.T) {
	db := setupClickHouseLogTest(t)
	stamp, user, quota, text := int64(1700000000), int64(4), int64(4294967296), "unchanged"
	rows := []ClickHouseLogArchiveRow{
		{ID: 11, CreatedAt: &stamp, UserID: &user, Quota: &quota, SessionID: &text},
		{ID: 12, CreatedAt: &stamp, UserID: &user, ChannelName: &text},
	}
	var input bytes.Buffer
	for _, row := range rows {
		encoded, err := common.Marshal(row)
		require.NoError(t, err)
		input.Write(encoded)
		input.WriteByte('\n')
	}
	hash := sha256.Sum256(input.Bytes())
	manifest := ClickHouseLogImportManifest{Source: "fixture", AfterID: 10, ThroughID: 12, Month: 202311, Rows: 2, SHA256: hex.EncodeToString(hash[:])}
	live := &Log{CreatedAt: stamp, UserId: 4, Quota: 9}
	require.NoError(t, createLog(live))
	result, err := ImportClickHouseLogs(context.Background(), db, manifest, bytes.NewReader(input.Bytes()))
	require.NoError(t, err)
	assert.False(t, result.AlreadyPublished)
	again, err := ImportClickHouseLogs(context.Background(), db, manifest, bytes.NewReader(input.Bytes()))
	require.NoError(t, err)
	assert.True(t, again.AlreadyPublished)
	assert.Equal(t, result.BatchID, again.BatchID)
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(3), count, "retry must not duplicate rows or replace the live partition")
	var nulls int64
	require.NoError(t, db.Raw("SELECT count() FROM logs WHERE migration_batch = ? AND isNull(request_id)", result.BatchID).Scan(&nulls).Error)
	assert.Equal(t, int64(2), nulls)
	bad := manifest
	bad.SHA256 = strings.Repeat("0", 64)
	_, err = ImportClickHouseLogs(context.Background(), db, bad, bytes.NewReader(input.Bytes()))
	require.ErrorContains(t, err, "SHA256")
	bad = manifest
	bad.AfterID = 0
	_, err = ImportClickHouseLogs(context.Background(), db, bad, bytes.NewReader(input.Bytes()))
	require.ErrorContains(t, err, "overlaps")
	changed := bytes.ReplaceAll(input.Bytes(), []byte("unchanged"), []byte("different"))
	changedHash := sha256.Sum256(changed)
	bad = manifest
	bad.SHA256 = hex.EncodeToString(changedHash[:])
	_, err = ImportClickHouseLogs(context.Background(), db, bad, bytes.NewReader(changed))
	require.ErrorContains(t, err, "differs")
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(3), count)
	var leftovers int64
	require.NoError(t, db.Raw("SELECT count() FROM system.tables WHERE database = currentDatabase() AND startsWith(name, 'logs_import_')").Scan(&leftovers).Error)
	assert.Zero(t, leftovers)
	_, err = ImportClickHouseLogs(context.Background(), db, manifest, io.MultiReader(bytes.NewReader(input.Bytes()), iotest.ErrReader(errors.New("archive interrupted"))))
	require.ErrorContains(t, err, "archive interrupted")
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(3), count)
	// Timestamp ordering can straddle source ID ranges. Disjoint actual IDs
	// must remain importable even when the numeric range overlaps.
	disjointRow := ClickHouseLogArchiveRow{ID: 13, CreatedAt: &stamp}
	disjointJSON, err := common.Marshal(disjointRow)
	require.NoError(t, err)
	disjointJSON = append(disjointJSON, '\n')
	disjointHash := sha256.Sum256(disjointJSON)
	disjointManifest := ClickHouseLogImportManifest{Source: "fixture", AfterID: 10, ThroughID: 13, Month: 202311, Rows: 1, SHA256: hex.EncodeToString(disjointHash[:])}
	_, err = ImportClickHouseLogs(context.Background(), db, disjointManifest, bytes.NewReader(disjointJSON))
	require.NoError(t, err)
}

func TestClickHouseLegacySchemaRequiresExplicitMigration(t *testing.T) {
	db := setupClickHouseLogTest(t)
	require.NoError(t, db.Exec("ALTER TABLE logs MODIFY COLUMN quota Nullable(Int32)").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, created_at, quota) VALUES (1, 1700000000, 15)").Error)
	require.ErrorContains(t, migrateClickHouseLogDB(), "schema incompatible")
	var quota int64
	require.NoError(t, db.Raw("SELECT quota FROM logs WHERE id = 1").Scan(&quota).Error)
	assert.Equal(t, int64(15), quota)
}

func TestClickHouseCursorRepeatedRequestAndUserIsolation(t *testing.T) {
	db := setupClickHouseLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs (created_at, user_id, request_id, event_id, other) VALUES
		(1700000000, 4, 'same', 'c', '{"admin_info":{"secret":"hidden"}}'),
		(1700000000, 4, 'same', 'b', '{}'),
		(1700000000, 4, 'same', 'a', '{}'),
		(1700000000, 5, 'same', 'other-user', '{}')`).Error)
	page, err := GetUserLogsCursor(context.Background(), 4, 0, 1, 1800000000, "", "", 2, "", "", "", "")
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "c", page.Items[0].EventID)
	assert.NotContains(t, page.Items[0].Other, "secret")
	assert.True(t, page.HasMore)
	// A new leading row must not shift the next page as it would with OFFSET.
	require.NoError(t, db.Exec("INSERT INTO logs (created_at, user_id, event_id) VALUES (1700000001, 4, 'new')").Error)
	next, err := GetUserLogsCursor(context.Background(), 4, 0, 1, 1800000000, "", "", 2, "", "", "", page.NextCursor)
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	assert.Equal(t, "a", next.Items[0].EventID)
	assert.False(t, next.HasMore)
	assert.Empty(t, next.NextCursor)
	_, err = GetUserLogsCursor(context.Background(), 4, 0, 1, 1800000000, "", "", 2, "", "", "", "not a cursor")
	assert.ErrorIs(t, err, ErrInvalidLogCursor)
}
