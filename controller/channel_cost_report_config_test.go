package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportConfigConfirmationAPI(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ChannelCostReportJob{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	root := model.User{Username: "config-root", Role: 100, Status: 1}
	require.NoError(t, db.Create(&root).Error)
	channel := model.Channel{Name: "configured", Key: "private-key", CostSettings: `{"mode":"manual","ratio":0.25}`}
	require.NoError(t, db.Create(&channel).Error)
	call := func(handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("id", root.Id)
		c.Set("role", 100)
		c.Request = httptest.NewRequest("POST", "/api/channel-finance/cost-preview", strings.NewReader(body))
		handler(c)
		return rec
	}
	preview := call(PreviewChannelCostReportConfig, "")
	require.Equal(t, http.StatusOK, preview.Code)
	assert.NotContains(t, preview.Body.String(), "private-key")
	assert.Equal(t, "no-store", preview.Header().Get("Cache-Control"))
	var response struct{ Data model.CostReportConfigPreview }
	require.NoError(t, common.Unmarshal(preview.Body.Bytes(), &response))
	require.Len(t, response.Data.Channels, 1)
	body := `{"request_key":"` + uuid.NewString() + `","start":100,"end":3700,"fallback_mode":"billed_basis","cost_fingerprint":"` + response.Data.Fingerprint + `"}`
	require.NoError(t, db.Model(&channel).Update("cost_settings", `{"mode":"manual","ratio":0.5}`).Error)
	changed := call(CreateChannelCostReportJob, body)
	assert.Equal(t, http.StatusConflict, changed.Code)
	var count int64
	require.NoError(t, db.Model(&model.ChannelCostReportJob{}).Count(&count).Error)
	assert.Zero(t, count, "stale confirmations cannot enqueue a job")
}
