package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// UserRouting handles only the administrator routes, never self-service data.
func UserRouting(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if c.GetInt("role") < common.RoleAdminUser || !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorI18n(c, i18n.MsgUserNoPermissionSameLevel)
		return
	}
	if c.Request.Method == http.MethodGet {
		rules, err := model.GetUserRoutingRules(id)
		if err != nil {
			common.SysError("read user routing rules: " + err.Error())
			common.ApiErrorI18n(c, i18n.MsgOperationFailed)
			return
		}
		common.ApiSuccess(c, rules)
		return
	}
	var request struct {
		Rules *[]model.UserRoutingRule `json:"rules"`
	}
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10), &request); err != nil || request.Rules == nil || !validUserRoutingRules(*request.Rules) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.SaveUserRoutingRules(id, *request.Rules); err != nil {
		common.SysError("save user routing rules: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgOperationFailed)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), "Updated user routing", c.ClientIP(), "user_routing_update", nil,
		map[string]interface{}{"target_user_id": id, "rules": *request.Rules}, nil)
	common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
	common.ApiSuccess(c, nil)
}

func validUserRoutingRules(rules []model.UserRoutingRule) bool {
	if len(rules) > 32 {
		return false
	}
	groups := ratio_setting.GetGroupRatioCopy()
	seen := make(map[[2]string]bool)
	for i := range rules {
		rule := &rules[i]
		rule.SourceGroup = strings.TrimSpace(rule.SourceGroup)
		rule.TargetGroup = strings.TrimSpace(rule.TargetGroup)
		rule.Model = strings.TrimSpace(rule.Model)
		if rule.TargetGroup == "" || rule.TargetGroup == "auto" || rule.SourceGroup == "auto" || len(rule.Model) > 200 || len(rule.SourceGroup) > 64 || len(rule.TargetGroup) > 64 {
			return false
		}
		if _, ok := groups[rule.TargetGroup]; !ok {
			return false
		}
		if rule.SourceGroup != "" {
			if _, ok := groups[rule.SourceGroup]; !ok {
				return false
			}
		}
		key := [2]string{rule.SourceGroup, rule.Model}
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
