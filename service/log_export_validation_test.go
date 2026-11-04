package service

import (
	"compress/gzip"
	"encoding/csv"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func TestClickHouseOfflineLogExport(t *testing.T) {
	dsn := os.Getenv("TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("TEST_CLICKHOUSE_DSN is required")
	}
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))
	admin, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	name := "offline_log_test_" + uuid.New().String()[:8]
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	db, err := gorm.Open(clickhouse.Open(u.String()), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldType := model.LOG_DB, common.LogDatabaseType()
	model.LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	t.Cleanup(func() {
		model.LOG_DB = oldDB
		common.SetLogDatabaseType(oldType)
		assert.NoError(t, admin.Exec("DROP DATABASE "+name+" SYNC").Error)
		for _, database := range []*gorm.DB{db, admin} {
			conn, err := database.DB()
			if assert.NoError(t, err) {
				assert.NoError(t, conn.Close())
			}
		}
	})
	require.NoError(t, model.CreateClickHouseLogTable(db, 0))
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,event_id,content,upstream_request_id,other) VALUES
		(42,1700000000,2,'a','start','match','{"cache_tokens":60,"cache_creation_tokens":30,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}'),
		(42,1700000001,2,'b','end','match','{"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}'),
		(43,1700000000,2,'c','other-user','match','{}'),(42,1700000002,2,'d','outside','match','{}'),
		(42,1700000000,5,'e','other-type','match','{}'),(42,1700000000,2,'f','other-request','other','{}')`).Error)
	task := &model.LogExportTask{Id: 62003, UserId: 42,
		Filters: `{"type":2,"start_timestamp":1700000000,"end_timestamp":1700000001,"upstream_request_id":"match"}`}
	count, size, err := (&logExportCenter{}).generateExport(task)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Positive(t, size)
	file, err := os.Open(filepath.Join(exportDir, "62003.csv.gz"))
	require.NoError(t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\uFEFF"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3)
	columns := make(map[string]int)
	for i, header := range rows[0] {
		columns[header] = i
	}
	for _, header := range []string{"详情", "缓存读取tokens", "缓存写入tokens"} {
		require.Contains(t, columns, header)
	}
	for i, want := range [][]string{{"start", "60", "30"}, {"end", "0", "15"}} {
		row := rows[i+1]
		assert.Equal(t, want, []string{row[columns["详情"]], row[columns["缓存读取tokens"]], row[columns["缓存写入tokens"]]})
	}
}

func TestOfflineLogExportCacheColumns(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))
	const userID = 62004
	logs := []*model.Log{
		{UserId: userID, CreatedAt: 150, Type: model.LogTypeConsume, Content: "cached", PromptTokens: 100, CompletionTokens: 5,
			Other: `{"cache_tokens":60,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5,"admin_info":{"secret":"private"}}`},
		{UserId: userID, CreatedAt: 151, Type: model.LogTypeConsume, Content: "uncached"},
		{UserId: userID + 1, CreatedAt: 150, Type: model.LogTypeConsume, Content: "other-user"},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)
	t.Cleanup(func() {
		assert.NoError(t, model.LOG_DB.Where("user_id IN ?", []int{userID, userID + 1}).Delete(&model.Log{}).Error)
	})
	task := &model.LogExportTask{Id: 62004, UserId: userID, Filters: `{"type":2,"start_timestamp":100,"end_timestamp":200}`}
	count, _, err := (&logExportCenter{}).generateExport(task)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	file, err := os.Open(filepath.Join(exportDir, "62004.csv.gz"))
	require.NoError(t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\uFEFF"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Len(t, rows[0], 14)
	assert.Equal(t, []string{"详情", "缓存读取tokens", "缓存写入tokens"}, rows[0][11:])
	assert.Equal(t, []string{"cached", "60", "15"}, rows[1][11:])
	assert.Equal(t, []string{"uncached", "0", "0"}, rows[2][11:])
	assert.Equal(t, []string{"100", "5"}, rows[1][5:7])
	assert.NotContains(t, string(data), "private")
}

func TestLogExportWorkerRejectsInvalidTimeRange(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))

	tests := []struct {
		name        string
		filtersJSON string
	}{
		{name: "空字符串", filtersJSON: ""},
		{name: "空 JSON", filtersJSON: `{}`},
		{name: "start 为零", filtersJSON: `{"start_timestamp":0,"end_timestamp":200}`},
		{name: "end 为零", filtersJSON: `{"start_timestamp":100,"end_timestamp":0}`},
		{name: "反向区间", filtersJSON: `{"start_timestamp":201,"end_timestamp":200}`},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &model.LogExportTask{Id: index + 1, Filters: test.filtersJSON}
			_, _, err := (&logExportCenter{}).generateExport(task)

			require.Error(t, err)
			assert.Equal(t, "invalid export time range", err.Error())
		})
	}
}

func TestExportFiltersAllowValidLongRange(t *testing.T) {
	filters, err := parseExportFilters(`{"start_timestamp":100,"end_timestamp":7776100}`)
	require.NoError(t, err)

	assert.NoError(t, validateExportFilters(filters))
}

func TestExportFiltersPreserveUpstreamRequestID(t *testing.T) {
	filters, err := parseExportFilters(`{"start_timestamp":100,"end_timestamp":200,"upstream_request_id":"upstream-match"}`)
	require.NoError(t, err)

	assert.Equal(t, "upstream-match", filters.UpstreamRequestId)
}

func TestLogExportWorkerAppliesTypeFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))
	require.NoError(t, model.DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM logs").Error
	})

	const userID = 62001
	logs := []*model.Log{
		{
			UserId:    userID,
			CreatedAt: 150,
			Type:      model.LogTypeManage,
			Content:   "included-manage-row",
		},
		{
			UserId:    userID,
			CreatedAt: 150,
			Type:      model.LogTypeConsume,
			Content:   "excluded-consume-row",
		},
	}
	require.NoError(t, model.DB.Create(&logs).Error)

	task := &model.LogExportTask{
		Id:      62002,
		UserId:  userID,
		Filters: `{"type":3,"start_timestamp":100,"end_timestamp":200}`,
	}
	rowCount, _, err := (&logExportCenter{}).generateExport(task)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount)

	file, err := os.Open(filepath.Join(exportDir, "62002.csv.gz"))
	require.NoError(t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer reader.Close()
	csvData, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(csvData), "included-manage-row")
	assert.NotContains(t, string(csvData), "excluded-consume-row")
}
