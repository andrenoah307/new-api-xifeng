package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func GetTokenUsageAlertStatus(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	status, err := service.TokenUsageAlertStatus(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, status)
}

func PreviewTokenUsageAlertEmail(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var request struct {
		Config setting.TokenUsageAlertConfig `json:"config"`
		Scope  string                        `json:"scope"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	raw, err := common.Marshal(request.Config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	config, err := setting.ParseTokenUsageAlerts(string(raw))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := "total"
	reset := int64(0)
	if request.Scope == "period" {
		scope = "period"
		now := time.Now().In(model.RevenueTimezone)
		reset = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, model.RevenueTimezone).Unix()
	}
	rule := setting.TokenUsageAlertRule{Percent: 50, CooldownDays: 7}
	if len(config.Rules) > 0 {
		rule = config.Rules[0]
	}
	subject, body, err := service.RenderTokenUsageAlertEmail(config, model.Token{Id: 123, Name: "示例 API Key"}, model.User{Username: "示例用户"}, model.TokenUsageAlertUsage{Scope: scope, Used: int64(max(80, rule.Percent)) * 10000, Limit: 1000000, ResetAt: reset}, rule)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"subject": subject, "body": "<p>预览示例数据，不发送邮件。</p>" + body})
}
