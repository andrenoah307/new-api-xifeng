package controller

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type autoGroupRuleRequest struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Enabled     bool                       `json:"enabled"`
	Priority    int                        `json:"priority"`
	TargetGroup string                     `json:"target_group"`
	MatchMode   string                     `json:"match_mode"`
	Conditions  []model.AutoGroupCondition `json:"conditions"`
}

var validMetrics = map[string]bool{
	"total_spend":          true,
	"recent_spend":         true,
	"yesterday_spend":      true,
	"total_topup":          true,
	"recent_topup":         true,
	"yesterday_topup":      true,
	"total_request_count":  true,
	"recent_request_count": true,
}

var validOps = map[string]bool{
	">=": true, ">": true, "<=": true, "<": true, "==": true, "!=": true,
}

func validateAutoGroupRuleRequest(req *autoGroupRuleRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	if strings.TrimSpace(req.TargetGroup) == "" {
		return fmt.Errorf("目标分组不能为空")
	}
	groups := ratio_setting.GetGroupRatioCopy()
	if _, ok := groups[req.TargetGroup]; !ok {
		return fmt.Errorf("目标分组 %s 不存在", req.TargetGroup)
	}
	if len(req.Conditions) == 0 {
		return fmt.Errorf("至少需要一个条件")
	}
	if req.MatchMode != "all" && req.MatchMode != "any" {
		req.MatchMode = "all"
	}
	for i, cond := range req.Conditions {
		if !validMetrics[cond.Metric] {
			return fmt.Errorf("条件 %d: 不支持的指标 %s", i+1, cond.Metric)
		}
		if !validOps[cond.Op] {
			return fmt.Errorf("条件 %d: 不支持的操作符 %s", i+1, cond.Op)
		}
		if strings.HasPrefix(cond.Metric, "recent_") && cond.Param <= 0 {
			return fmt.Errorf("条件 %d: recent 类指标的参数 (小时数) 必须大于 0", i+1)
		}
	}
	return nil
}

func GetAutoGroupRules(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	rules, total, err := model.ListAutoGroupRules(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rules)
	common.ApiSuccess(c, pageInfo)
}

func GetAutoGroupRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil || ruleID <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	rule, err := model.GetAutoGroupRuleByID(ruleID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func CreateAutoGroupRule(c *gin.Context) {
	var req autoGroupRuleRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数")
		return
	}
	if err := validateAutoGroupRuleRequest(&req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	conditionsJSON, err := common.Marshal(req.Conditions)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule := &model.AutoGroupRule{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
		TargetGroup: req.TargetGroup,
		MatchMode:   req.MatchMode,
		Conditions:  string(conditionsJSON),
		CreatedBy:   c.GetInt("id"),
		UpdatedBy:   c.GetInt("id"),
	}
	if err := model.CreateAutoGroupRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	service.ReloadAutoGroupRules()
	common.ApiSuccess(c, rule)
}

func UpdateAutoGroupRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil || ruleID <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	var req autoGroupRuleRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数")
		return
	}
	if err := validateAutoGroupRuleRequest(&req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	conditionsJSON, err := common.Marshal(req.Conditions)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule := &model.AutoGroupRule{
		Id:          ruleID,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
		TargetGroup: req.TargetGroup,
		MatchMode:   req.MatchMode,
		Conditions:  string(conditionsJSON),
		UpdatedBy:   c.GetInt("id"),
	}
	if err := model.UpdateAutoGroupRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	service.ReloadAutoGroupRules()
	common.ApiSuccess(c, rule)
}

func DeleteAutoGroupRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil || ruleID <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	if err := model.DeleteAutoGroupRule(ruleID); err != nil {
		common.ApiError(c, err)
		return
	}
	service.ReloadAutoGroupRules()
	common.ApiSuccess(c, gin.H{"id": ruleID})
}

type autoGroupEnrollRequest struct {
	UserIds []int `json:"user_ids"`
}

func GetAutoGroupEnrollments(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	enrollments, total, err := model.ListAutoGroupEnrollments(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(enrollments)
	common.ApiSuccess(c, pageInfo)
}

func CreateAutoGroupEnrollment(c *gin.Context) {
	var req autoGroupEnrollRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数")
		return
	}
	if len(req.UserIds) == 0 {
		common.ApiErrorMsg(c, "至少需要一个用户 ID")
		return
	}
	adminId := c.GetInt("id")
	var created []*model.AutoGroupEnrollment
	var errors []string

	for _, userId := range req.UserIds {
		user, err := model.GetUserById(userId, false)
		if err != nil {
			errors = append(errors, fmt.Sprintf("用户 %d 不存在", userId))
			continue
		}
		existing, _ := model.GetEnrollmentByUserId(userId)
		if existing != nil {
			errors = append(errors, fmt.Sprintf("用户 %d 已被纳入", userId))
			continue
		}
		enrollment := &model.AutoGroupEnrollment{
			UserId:        userId,
			OriginalGroup: user.Group,
			CurrentGroup:  user.Group,
			CurrentRuleId: 0,
			EnrolledBy:    adminId,
		}
		if err := model.CreateAutoGroupEnrollment(enrollment); err != nil {
			errors = append(errors, fmt.Sprintf("纳入用户 %d 失败: %v", userId, err))
			continue
		}
		created = append(created, enrollment)
	}

	service.ReloadAutoGroupEnrollments()
	for _, e := range created {
		service.NotifyAutoGroupEvaluation(e.UserId)
	}

	result := gin.H{
		"created": created,
		"count":   len(created),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}
	common.ApiSuccess(c, result)
}

func DeleteAutoGroupEnrollment(c *gin.Context) {
	enrollmentID, err := strconv.Atoi(c.Param("id"))
	if err != nil || enrollmentID <= 0 {
		common.ApiErrorMsg(c, "无效的纳入 ID")
		return
	}
	enrollment, err := model.DeleteAutoGroupEnrollment(enrollmentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.ReloadAutoGroupEnrollments()
	common.ApiSuccess(c, enrollment)
}

func TriggerAutoGroupSweep(c *gin.Context) {
	service.TriggerAutoGroupSweep()
	common.ApiSuccess(c, gin.H{"message": "全量扫描已触发"})
}
