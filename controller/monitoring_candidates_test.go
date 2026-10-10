package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringCandidatesResponse(t *testing.T) {
	t.Cleanup(operation_setting.RebuildMonitoringExclusionIndex)
	db := setupOptionControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	require.NoError(t, db.Create(&model.Channel{Id: 7, Name: "candidate-name", Key: "private-channel-key"}).Error)
	require.NoError(t, model.UpdateOption("group_monitoring_setting.excluded_channel_ids", "[7,999]"))
	for _, query := range []string{"kind=channel&keyword=candidate", "kind=channel&selected=true"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		GetMonitoringCandidates(c)
		assert.JSONEq(t, `{"success":true,"message":"","data":[{"id":7,"name":"candidate-name"}]}`, w.Body.String())
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?kind=invalid", nil)
	GetMonitoringCandidates(c)
	assert.JSONEq(t, `{"success":false,"message":"无效的参数"}`, w.Body.String())
}
