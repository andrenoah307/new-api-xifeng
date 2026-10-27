package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

func TestClickHouseUserCursorHTTP(t *testing.T) {
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
	require.NoError(t, db.Exec(model.ClickHouseLogCreateTableSQL(0)).Error)
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
