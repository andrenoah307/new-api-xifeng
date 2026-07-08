package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type AutoGroupRule struct {
	Id          int            `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"type:varchar(128);uniqueIndex"`
	Description string         `json:"description" gorm:"type:text"`
	Enabled     bool           `json:"enabled" gorm:"default:false;index"`
	Priority    int            `json:"priority" gorm:"default:0;index"`
	TargetGroup string         `json:"target_group" gorm:"type:varchar(64)"`
	MatchMode   string         `json:"match_mode" gorm:"type:varchar(8);default:'all'"`
	Conditions  string         `json:"conditions" gorm:"type:text"`
	CreatedAt   int64          `json:"created_at" gorm:"bigint;index"`
	UpdatedAt   int64          `json:"updated_at" gorm:"bigint"`
	CreatedBy   int            `json:"created_by" gorm:"default:0"`
	UpdatedBy   int            `json:"updated_by" gorm:"default:0"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

type AutoGroupCondition struct {
	Metric   string  `json:"metric"`
	Op       string  `json:"op"`
	Value    float64 `json:"value"`
	ValueStr string  `json:"value_str,omitempty"`
	Param    int     `json:"param,omitempty"`
}

func (rule *AutoGroupRule) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if rule.CreatedAt == 0 {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	return nil
}

func (rule *AutoGroupRule) BeforeUpdate(tx *gorm.DB) error {
	rule.UpdatedAt = common.GetTimestamp()
	return nil
}

func (rule *AutoGroupRule) ParsedConditions() []AutoGroupCondition {
	if rule == nil {
		return nil
	}
	raw := strings.TrimSpace(rule.Conditions)
	if raw == "" {
		return nil
	}
	var conditions []AutoGroupCondition
	if err := common.UnmarshalJsonStr(raw, &conditions); err != nil {
		return nil
	}
	return conditions
}

func ListAutoGroupRules(offset, limit int, keyword string) ([]*AutoGroupRule, int64, error) {
	var rules []*AutoGroupRule
	var total int64
	tx := DB.Model(&AutoGroupRule{})
	if keyword != "" {
		tx = tx.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("priority desc, id asc").Offset(offset).Limit(limit).Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

func ListEnabledAutoGroupRules() ([]*AutoGroupRule, error) {
	var rules []*AutoGroupRule
	err := DB.Where("enabled = ?", true).Order("priority desc, id asc").Find(&rules).Error
	return rules, err
}

func GetAutoGroupRuleByID(id int) (*AutoGroupRule, error) {
	if id <= 0 {
		return nil, errors.New("invalid rule id")
	}
	var rule AutoGroupRule
	if err := DB.First(&rule, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func CreateAutoGroupRule(rule *AutoGroupRule) error {
	if rule == nil {
		return errors.New("rule is nil")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	return DB.Create(rule).Error
}

func UpdateAutoGroupRule(rule *AutoGroupRule) error {
	if rule == nil || rule.Id <= 0 {
		return errors.New("invalid rule")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	return DB.Model(&AutoGroupRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
		"name":         rule.Name,
		"description":  rule.Description,
		"enabled":      rule.Enabled,
		"priority":     rule.Priority,
		"target_group": rule.TargetGroup,
		"match_mode":   rule.MatchMode,
		"conditions":   rule.Conditions,
		"updated_at":   common.GetTimestamp(),
		"updated_by":   rule.UpdatedBy,
	}).Error
}

func DeleteAutoGroupRule(id int) error {
	if id <= 0 {
		return errors.New("invalid rule id")
	}
	return DB.Delete(&AutoGroupRule{}, "id = ?", id).Error
}
