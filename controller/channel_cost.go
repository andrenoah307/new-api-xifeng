package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetChannelCost(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, channel.GetCostSettings())
}

func UpdateChannelCost(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	var settings model.ChannelCostSettings
	if err != nil || id <= 0 || common.DecodeJson(c.Request.Body, &settings) != nil || settings.Validate() != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	if channel.GetCostSettings().Version != settings.Version {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	if settings.Mode == "auto" {
		declaration, err := service.FetchChannelCostRatio(c.Request.Context(), channel, settings)
		if err != nil {
			logRootUnavailable(c, i18n.MsgInvalidParams)
			return
		}
		settings.Ratio, settings.Upstream = declaration.ResolvedRate, declaration
		settings.SyncStatus, settings.LastAttemptAt = "ok", time.Now().Unix()
	} else {
		settings.Upstream, settings.SyncStatus, settings.LastAttemptAt = nil, "", 0
	}
	settings.UpdatedAt = time.Now().Unix()
	if !authorizeLogRoot(c) {
		return
	}
	if err := service.SaveChannelCost(c.Request.Context(), channel, settings); err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	model.InitChannelCache()
	GetChannelCost(c)
}
