package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func GetBotLogs(c *gin.Context) {
	for name, values := range c.Request.URL.Query() {
		switch name {
		case "username", "model_name", "token_name", "group", "request_id", "type", "start_timestamp", "end_timestamp", "page_size", "pagination", "p", "cursor":
			if len(values) == 1 {
				continue
			}
		}
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	filter := model.BotLogFilter{Username: c.Query("username"), ModelName: c.Query("model_name"),
		TokenName: c.Query("token_name"), Group: c.Query("group"), RequestID: c.Query("request_id")}
	var err error
	filter.Type, err = strconv.Atoi(c.DefaultQuery("type", "0"))
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	filter.Start, err = strconv.ParseInt(c.DefaultQuery("start_timestamp", "0"), 10, 64)
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	filter.End, err = strconv.ParseInt(c.DefaultQuery("end_timestamp", "0"), 10, 64)
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	filter.PageSize, err = strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if err != nil || (c.Query("pagination") != "" && c.Query("pagination") != "cursor") || (c.Query("p") != "" && c.Query("p") != "1") {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	ctx, cancel := newLogQueryContext(c.Request.Context())
	defer cancel()
	page, err := model.GetBotLogs(ctx, c.GetInt("id"), filter, c.Query("cursor"))
	if err != nil {
		if errors.Is(err, model.ErrInvalidLogCursor) {
			logRootUnavailable(c, i18n.MsgInvalidParams)
		} else {
			// SQL errors can contain private filter values; never return them.
			logRootUnavailable(c, i18n.MsgDatabaseError)
		}
		return
	}
	// Recheck revocation after a potentially slow log query.
	user, err := model.ValidateAccessToken(c.GetString("bot_system_token"))
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	if user == nil || user.Id != c.GetInt("id") || user.Role != common.RoleBotUser || user.Status != common.UserStatusEnabled || !authz.Can(user.Id, user.Role, authz.LogsReadRedacted) {
		logRootUnavailable(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}
	common.ApiSuccess(c, page)
}
