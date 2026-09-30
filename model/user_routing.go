package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// Rules are ordered: the first enabled exact match wins. Empty source/model
// matches any value. Policies live outside users so self-service cannot edit them.
type UserRoutingRule struct {
	SourceGroup string `json:"source_group"`
	Model       string `json:"model"`
	TargetGroup string `json:"target_group"`
	Enabled     bool   `json:"enabled"`
}

type UserRoutingPolicy struct {
	UserID int    `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	Rules  string `gorm:"type:text;not null" json:"-"`
}

var userRoutingCache = struct {
	sync.RWMutex
	policies map[int][]UserRoutingRule
}{policies: make(map[int][]UserRoutingRule)}

// Serializes refresh and saves so an older database read cannot replace a save.
var userRoutingWriteMu sync.Mutex

func GetUserRoutingRules(userID int) ([]UserRoutingRule, error) {
	var policies []UserRoutingPolicy
	if err := DB.Where("user_id = ?", userID).Find(&policies).Error; err != nil {
		return nil, err
	}
	rules := []UserRoutingRule{}
	if len(policies) > 0 {
		if err := common.UnmarshalJsonStr(policies[0].Rules, &rules); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func CachedUserRoutingRules(userID int) []UserRoutingRule {
	userRoutingCache.RLock()
	defer userRoutingCache.RUnlock()
	return append([]UserRoutingRule(nil), userRoutingCache.policies[userID]...)
}

func SaveUserRoutingRules(userID int, rules []UserRoutingRule) error {
	data, err := common.Marshal(rules)
	if err != nil {
		return err
	}
	userRoutingWriteMu.Lock()
	defer userRoutingWriteMu.Unlock()
	if len(rules) == 0 {
		err = DB.Where("user_id = ?", userID).Delete(&UserRoutingPolicy{}).Error
	} else {
		err = DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"rules"})}).Create(&UserRoutingPolicy{UserID: userID, Rules: string(data)}).Error
	}
	if err != nil {
		return err
	}
	userRoutingCache.Lock()
	defer userRoutingCache.Unlock()
	if len(rules) == 0 {
		delete(userRoutingCache.policies, userID)
	} else {
		userRoutingCache.policies[userID] = append([]UserRoutingRule(nil), rules...)
	}
	return nil
}

func RefreshUserRoutingPolicies() error {
	userRoutingWriteMu.Lock()
	defer userRoutingWriteMu.Unlock()
	var policies []UserRoutingPolicy
	if err := DB.Find(&policies).Error; err != nil {
		return err
	}
	updated := make(map[int][]UserRoutingRule, len(policies))
	for _, policy := range policies {
		var rules []UserRoutingRule
		if err := common.UnmarshalJsonStr(policy.Rules, &rules); err != nil {
			return fmt.Errorf("invalid user routing policy %d: %w", policy.UserID, err)
		}
		updated[policy.UserID] = rules
	}
	userRoutingCache.Lock()
	userRoutingCache.policies = updated
	userRoutingCache.Unlock()
	return nil
}

func SyncUserRoutingPolicies() {
	for range time.NewTicker(30 * time.Second).C {
		if err := RefreshUserRoutingPolicies(); err != nil {
			// Keep the last complete snapshot; never silently remove routing on failure.
			common.SysError("user routing policy refresh failed: " + err.Error())
		}
	}
}
