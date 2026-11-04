package controller

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func setupClickHouseHTTPLogTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("TEST_CLICKHOUSE_DSN is required")
	}
	setupLogExportValidationDB(t)
	admin, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	name := "http_log_test_" + uuid.New().String()[:8]
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
	return db
}

func TestClickHouseOnlineLogExports(t *testing.T) {
	db := setupClickHouseHTTPLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,type,event_id,content,other) VALUES
		(42,1700000000,2,'a','start','{"cache_tokens":60,"cache_creation_tokens":30,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}'),
		(42,1700000001,2,'b','end','{"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}'),
		(43,1700000000,2,'c','other-user','{}'),(42,1700000002,2,'d','outside','{}'),
		(42,1700000000,5,'e','other-type','{}')`).Error)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		want    []string
	}{
		{"admin", ExportAllLogsCsv, []string{"start", "other-user", "end"}},
		{"user", ExportUserLogsCsv, []string{"start", "end"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := performLogExportRequest(t, tc.handler, "/?type=2&start_timestamp=1700000000&end_timestamp=1700000001")
			require.Equal(t, http.StatusOK, recorder.Code)
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(recorder.Body.String(), "\uFEFF"))).ReadAll()
			require.NoError(t, err)
			require.Len(t, rows, len(tc.want)+1)
			columns := make(map[string]int)
			for i, header := range rows[0] {
				columns[header] = i
			}
			for _, header := range []string{"详情", "缓存读取tokens", "缓存写入tokens"} {
				require.Contains(t, columns, header)
			}
			cacheCounts := map[string][]string{
				"start": {"60", "30"}, "end": {"0", "15"}, "other-user": {"0", "0"},
			}
			var contents []string
			for _, row := range rows[1:] {
				content := row[columns["详情"]]
				contents = append(contents, content)
				assert.Equal(t, cacheCounts[content], []string{row[columns["缓存读取tokens"]], row[columns["缓存写入tokens"]]})
			}
			assert.Equal(t, tc.want, contents)
		})
	}
}

func TestClickHouseUserCursorHTTP(t *testing.T) {
	db := setupClickHouseHTTPLogTest(t)
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id,created_at,event_id,content,other) VALUES
		(4,1700000000,'b','first','{"admin_info":{"secret":"hidden"}}'),
		(4,1700000000,'a','second','{}'),(5,1700000000,'z','private','{}')`).Error)
	cursor := ""
	for _, want := range []string{"first", "second"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("id", 4)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?pagination=cursor&start_timestamp=1&page_size=1&cursor="+url.QueryEscape(cursor), nil)
		GetUserLogs(c)
		var response struct {
			Success bool
			Data    model.LogCursorPage
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
		require.True(t, response.Success, rec.Body.String())
		require.Len(t, response.Data.Items, 1)
		assert.Equal(t, want, response.Data.Items[0].Content)
		assert.NotContains(t, rec.Body.String(), "hidden")
		assert.NotContains(t, rec.Body.String(), "event_id")
		cursor = response.Data.NextCursor
	}
	assert.Empty(t, cursor)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("id", 4)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?pagination=cursor&cursor=invalid", nil)
	GetUserLogs(c)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.NotContains(t, rec.Body.String(), "SELECT")
}
