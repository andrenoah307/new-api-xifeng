package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootLogBatchLimitsAndAuthorization(t *testing.T) {
	t.Setenv("ROOT_LOG_PAGE_SIZE", "10000")
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "root-log-batch", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	logs := make([]model.Log, 205)
	for i := range logs {
		logs[i] = model.Log{CreatedAt: 100, Type: 2, Quota: 1}
	}
	require.NoError(t, db.CreateInBatches(logs, 50).Error)
	for _, tt := range []struct {
		name  string
		role  int
		query string
		size  int
		count int
		total int64
	}{
		{"root bulk", common.RoleRootUser, "page_size=1000", 1000, 205, 205},
		{"root bounded", common.RoleRootUser, "page_size=999999", 10000, 205, 205},
		{"admin unchanged", common.RoleAdminUser, "page_size=1000", 100, 100, 205},
		{"root no count", common.RoleRootUser, "page_size=1000&count=false", 1000, 205, -1},
		{"root cursor", common.RoleRootUser, "page_size=1000&before_id=101&count=false", 1000, 100, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?start_timestamp=1&end_timestamp=200&"+tt.query, nil)
			c.Set("id", user.Id)
			c.Set("role", tt.role)
			GetAllLogs(c)
			var response struct {
				Success bool
				Data    struct {
					PageSize int `json:"page_size"`
					Total    int64
					Items    []model.Log
				}
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
			require.True(t, response.Success, rec.Body.String())
			assert.Equal(t, tt.size, response.Data.PageSize)
			assert.Equal(t, tt.total, response.Data.Total)
			assert.Len(t, response.Data.Items, tt.count)
		})
	}
	seen := make(map[int]bool)
	cursor := "0"
	for page := 0; page < 3; page++ {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?start_timestamp=1&end_timestamp=200&page_size=100&count=false&before_id="+cursor, nil)
		c.Set("id", user.Id)
		c.Set("role", common.RoleRootUser)
		GetAllLogs(c)
		var response struct {
			Success bool
			Data    struct {
				Items   []model.Log
				HasMore bool   `json:"has_more"`
				Next    string `json:"next_before_id"`
			}
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
		require.True(t, response.Success, rec.Body.String())
		assert.Equal(t, page < 2, response.Data.HasMore)
		for _, log := range response.Data.Items {
			require.False(t, seen[log.Id], "cursor must not repeat a ledger row")
			seen[log.Id] = true
		}
		cursor = response.Data.Next
		if page < 2 {
			assert.Equal(t, strconv.Itoa(response.Data.Items[len(response.Data.Items)-1].Id), cursor)
		} else {
			assert.Equal(t, "0", cursor)
		}
	}
	assert.Len(t, seen, 205)
	require.NoError(t, db.Model(&user).Update("role", common.RoleAdminUser).Error)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/?start_timestamp=1&page_size=1000&count=false", nil)
	c.Set("id", user.Id)
	c.Set("role", common.RoleRootUser)
	GetAllLogs(c)
	assert.NotContains(t, rec.Body.String(), `"success":true`)
}

func TestRootFinanceStatsAuthenticationRangesAndCache(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	root := model.User{Username: "root-finance-stat", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&model.Log{CreatedAt: 100, Type: 2, ChannelId: 7, Quota: 200, Other: `{"group_ratio":2}`}).Error)
	call := func(role int, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Set("id", root.Id)
		c.Set("role", role)
		GetChannelFinanceStats(c)
		return rec
	}
	path := "/api/log/finance/stat?start_timestamp=1&end_timestamp=200"
	denied := call(common.RoleAdminUser, path)
	assert.Equal(t, http.StatusServiceUnavailable, denied.Code)
	result := call(common.RoleRootUser, path)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	var response struct {
		Success bool
		Data    channelFinanceSnapshot
	}
	require.NoError(t, common.Unmarshal(result.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Channels, 1)
	assert.Equal(t, "100", response.Data.Channels[0].KnownModelQuota)
	assert.Equal(t, "no-store", result.Header().Get("Cache-Control"))
	assert.True(t, response.Data.Complete)
	assert.True(t, response.Data.RefundCostResolved)
	segmented := call(common.RoleRootUser, "/api/log/finance/stat?start_timestamp=1&end_timestamp=10000&window_seconds=100")
	require.NoError(t, common.Unmarshal(segmented.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.False(t, response.Data.Complete)
	assert.EqualValues(t, 101, response.Data.NextStart)
	assert.EqualValues(t, 10000, response.Data.RequestedEnd)
	assert.EqualValues(t, 101, response.Data.End)
	resumed := call(common.RoleRootUser, "/api/log/finance/stat?start_timestamp=101&end_timestamp=200&window_seconds=100")
	require.NoError(t, common.Unmarshal(resumed.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.True(t, response.Data.Complete)
	assert.Zero(t, response.Data.NextStart)
	assert.Empty(t, response.Data.Channels, "resume must not repeat the preceding window's consumption")
	// Restore the first range to the bounded single-entry cache.
	result = call(common.RoleRootUser, path)
	require.NoError(t, db.Exec("DELETE FROM logs").Error)
	cached := call(common.RoleRootUser, path)
	assert.JSONEq(t, result.Body.String(), cached.Body.String())
	uncached := call(common.RoleRootUser, path+"&channel=8")
	require.NoError(t, common.Unmarshal(uncached.Body.Bytes(), &response))
	assert.Empty(t, response.Data.Channels)
	assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, "/api/log/finance/stat?start_timestamp=0&end_timestamp=1").Code)
	require.NoError(t, db.Model(&root).Update("status", common.UserStatusDisabled).Error)
	assert.Equal(t, http.StatusServiceUnavailable, call(common.RoleRootUser, path).Code, "cached statistics must not bypass current role/status checks")
}
