package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func GetMonitoringCandidates(c *gin.Context) {
	kind := c.Query("kind")
	keyword := c.Query("keyword")
	if (kind != "channel" && kind != "user") || len(keyword) > 200 {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	selected := c.Query("selected") == "true"
	var ids []int
	if selected {
		cfg := operation_setting.GetGroupMonitoringSetting()
		ids = cfg.ExcludedChannelIDs
		if kind == "user" {
			ids = cfg.ExcludedUserIDs
		}
	}
	rows, err := model.MonitoringCandidates(kind == "user", keyword, ids, selected)
	if err != nil {
		common.ApiErrorMsg(c, "查询失败")
		return
	}
	common.ApiSuccess(c, rows)
}
