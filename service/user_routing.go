package service

import (
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const userRoutingSnapshotKey = "user_routing_snapshot"

func userRoutingRules(c *gin.Context) []model.UserRoutingRule {
	if c == nil {
		return nil
	}
	if snapshot, ok := c.Get(userRoutingSnapshotKey); ok {
		return snapshot.([]model.UserRoutingRule)
	}
	rules := model.CachedUserRoutingRules(c.GetInt("id"))
	c.Set(userRoutingSnapshotKey, rules)
	return rules
}

func UserRoutingTargetGroup(c *gin.Context, sourceGroup, modelName string) string {
	for _, rule := range userRoutingRules(c) {
		if rule.Enabled && (rule.SourceGroup == "" || rule.SourceGroup == sourceGroup) && (rule.Model == "" || rule.Model == modelName) {
			return rule.TargetGroup
		}
	}
	return sourceGroup
}

func SetUserRoutingSelection(c *gin.Context, sourceGroup, modelName string) {
	if c == nil {
		return
	}
	target := UserRoutingTargetGroup(c, sourceGroup, modelName)
	var info *dto.UserRoutingInfo
	if target != sourceGroup {
		info = &dto.UserRoutingInfo{SourceGroup: sourceGroup, TargetGroup: target, Model: modelName}
	}
	common.SetContextKey(c, constant.ContextKeyUserRouting, info)
}

func RequestAccountingGroup(c *gin.Context) string {
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "auto" {
		if resolved := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); resolved != "" {
			return resolved
		}
	}
	return group
}

// Isolate effective routes, including auto's candidates, without disturbing
// affinity for models/groups whose channel selection remains unchanged.
func userRoutingAffinityScope(c *gin.Context, modelName, usingGroup string) string {
	if len(userRoutingRules(c)) == 0 {
		return ""
	}
	groups := []string{usingGroup}
	if usingGroup == "auto" {
		groups = GetUserAutoGroup(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	routes := []dto.UserRoutingInfo{}
	for _, group := range groups {
		if target := UserRoutingTargetGroup(c, group, modelName); target != group {
			routes = append(routes, dto.UserRoutingInfo{SourceGroup: group, TargetGroup: target, Model: modelName})
		}
	}
	if len(routes) == 0 {
		return ""
	}
	data, _ := common.Marshal(routes)
	return fmt.Sprintf(":user-route:%d:%x", c.GetInt("id"), sha256.Sum256(data))
}
