package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type riskRuleUpsertRequest struct {
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	Enabled             bool                  `json:"enabled"`
	Scope               string                `json:"scope"`
	Detector            string                `json:"detector"`
	MatchMode           string                `json:"match_mode"`
	Priority            int                   `json:"priority"`
	Action              string                `json:"action"`
	AutoBlock           bool                  `json:"auto_block"`
	AutoRecover         bool                  `json:"auto_recover"`
	RecoverMode         string                `json:"recover_mode"`
	RecoverAfterSeconds int                   `json:"recover_after_seconds"`
	ResponseStatusCode  int                   `json:"response_status_code"`
	ResponseMessage     string                `json:"response_message"`
	ScoreWeight         int                   `json:"score_weight"`
	Conditions          []types.RiskCondition `json:"conditions"`
}

func GetRiskCenterOverview(c *gin.Context) {
	overview, err := service.GetRiskOverview()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, overview)
}

func GetRiskCenterConfig(c *gin.Context) {
	common.ApiSuccess(c, service.GetRiskControlConfig())
}

func UpdateRiskCenterConfig(c *gin.Context) {
	var req operation_setting.RiskControlSetting
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	operation_setting.NormalizeRiskControlSetting(&req)
	configMap, err := config.ConfigToMap(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for key, value := range configMap {
		if err = model.UpdateOption("risk_control."+key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, service.GetRiskControlConfig())
}

func DetectRiskIP(c *gin.Context) {
	common.ApiSuccess(c, requestip.DiagnoseRequest(c))
}

func GetRiskRules(c *gin.Context) {
	rules, err := service.ListRiskRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rules)
}

func CreateRiskRule(c *gin.Context) {
	rule, err := bindRiskRuleRequest(c, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule.CreatedBy = c.GetInt("id")
	rule.UpdatedBy = c.GetInt("id")
	if err = service.CreateRiskRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func UpdateRiskRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil || ruleID <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	rule, err := bindRiskRuleRequest(c, ruleID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rule.UpdatedBy = c.GetInt("id")
	if err = service.UpdateRiskRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func DeleteRiskRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil || ruleID <= 0 {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	if err = service.DeleteRiskRule(ruleID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": ruleID})
}

func GetRiskSubjects(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.RiskSubjectQuery{
		Scope:   c.Query("scope"),
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
	}
	items, total, err := service.ListRiskSubjectSnapshots(query, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetRiskIncidents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	query := model.RiskIncidentQuery{
		Scope:   c.Query("scope"),
		Action:  c.Query("action"),
		Keyword: c.Query("keyword"),
	}
	items, total, err := service.ListRiskIncidents(query, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func UnblockRiskSubject(c *gin.Context) {
	scope := c.Param("scope")
	subjectID, err := strconv.Atoi(c.Param("id"))
	if err != nil || subjectID <= 0 {
		common.ApiErrorMsg(c, "无效的主体 ID")
		return
	}
	operator := fmt.Sprintf("%s#%d", c.GetString("username"), c.GetInt("id"))
	if err = service.UnblockRiskSubject(scope, subjectID, operator); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"scope": scope,
		"id":    subjectID,
	})
}

func bindRiskRuleRequest(c *gin.Context, ruleID int) (*model.RiskRule, error) {
	var req riskRuleUpsertRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		return nil, err
	}
	conditionsBytes, err := common.Marshal(req.Conditions)
	if err != nil {
		return nil, err
	}
	return &model.RiskRule{
		Id:                  ruleID,
		Name:                req.Name,
		Description:         req.Description,
		Enabled:             req.Enabled,
		Scope:               req.Scope,
		Detector:            req.Detector,
		MatchMode:           req.MatchMode,
		Priority:            req.Priority,
		Action:              req.Action,
		AutoBlock:           req.AutoBlock,
		AutoRecover:         req.AutoRecover,
		RecoverMode:         req.RecoverMode,
		RecoverAfterSeconds: req.RecoverAfterSeconds,
		ResponseStatusCode:  req.ResponseStatusCode,
		ResponseMessage:     req.ResponseMessage,
		ScoreWeight:         req.ScoreWeight,
		Conditions:          string(conditionsBytes),
	}, nil
}
