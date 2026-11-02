package model

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TokenUsageAlertCapacity = 10000

// One rolling delivery record per user/token/rule, never per request or period.
type TokenUsageAlertDelivery struct {
	ID            string `gorm:"type:varchar(100);primaryKey"`
	UserID        int
	TokenID       int
	RuleID        string `gorm:"type:varchar(48)"`
	Status        string `gorm:"type:varchar(16);index"`
	CooldownUntil int64  `gorm:"type:bigint;index"`
	NextAttemptAt int64  `gorm:"type:bigint"`
	UpdatedAt     int64  `gorm:"type:bigint;autoUpdateTime:false"`
	Attempts      int
}

type TokenUsageAlertBudget struct {
	ID   int `gorm:"primaryKey;autoIncrement:false"`
	Rows int `gorm:"column:entry_count"`
}

type TokenUsageAlertUsage struct {
	UserID      int    `json:"user_id"`
	TokenID     int    `json:"token_id"`
	Scope       string `json:"scope"`
	Used        int64  `json:"used"`
	Limit       int64  `json:"limit"`
	PeriodStart int64  `json:"period_start"`
	ResetAt     int64  `json:"reset_at"`
}

var tokenUsageAlertObserver atomic.Pointer[func(int)]

func SetTokenUsageAlertObserver(observer func(int)) { tokenUsageAlertObserver.Store(&observer) }

func NotifyTokenUsageAlert(id int) {
	if !setting.TokenUsageAlertsEnabled() {
		return
	}
	if observer := tokenUsageAlertObserver.Load(); observer != nil {
		(*observer)(id)
	}
}

func TokenUsageAlertUsages(token Token, now time.Time) []TokenUsageAlertUsage {
	if token.UserId <= 0 || token.Id <= 0 || (token.Status != common.TokenStatusEnabled && token.Status != common.TokenStatusExhausted) || (token.ExpiredTime > 0 && token.ExpiredTime <= now.Unix()) {
		return nil
	}
	var usages []TokenUsageAlertUsage
	limit := int64(token.UsedQuota) + int64(token.RemainQuota)
	if !token.UnlimitedQuota && limit > 0 && token.UsedQuota >= 0 {
		usages = append(usages, TokenUsageAlertUsage{UserID: token.UserId, TokenID: token.Id, Scope: "total", Used: int64(token.UsedQuota), Limit: limit})
	}
	if token.PeriodLimitEnabled() {
		state := TokenPeriodState{Type: token.PeriodType, Days: token.PeriodDays, Limit: token.PeriodQuotaLimit, AnchorAt: token.PeriodAnchorAt, StartAt: token.PeriodStartAt, UsedQuota: token.PeriodUsedQuota}
		start := state.CurrentStart(now)
		if start > 0 {
			usages = append(usages, TokenUsageAlertUsage{UserID: token.UserId, TokenID: token.Id, Scope: "period", Used: state.EffectiveUsed(now), Limit: state.Limit, PeriodStart: start, ResetAt: state.ResetAt(now)})
		}
	}
	return usages
}

// Use decimal comparison: cycle limits may exceed the safe int64 * 100 range.
func MatchTokenUsageAlert(usages []TokenUsageAlertUsage, rule setting.TokenUsageAlertRule) (TokenUsageAlertUsage, bool) {
	var selected TokenUsageAlertUsage
	var ratio decimal.Decimal
	for _, usage := range usages {
		if usage.Limit <= 0 || usage.Used < 0 || (rule.Scope != "any" && rule.Scope != usage.Scope) {
			continue
		}
		current := decimal.NewFromInt(usage.Used).Mul(decimal.NewFromInt(100))
		if current.LessThan(decimal.NewFromInt(usage.Limit).Mul(decimal.NewFromInt(int64(rule.Percent)))) {
			continue
		}
		current = current.Div(decimal.NewFromInt(usage.Limit))
		if selected.Limit == 0 || current.GreaterThan(ratio) {
			selected, ratio = usage, current
		}
	}
	return selected, selected.Limit > 0
}

func TokenUsageAlertID(userID, tokenID int, ruleID string) string {
	return fmt.Sprintf("%d:%d:%s", userID, tokenID, ruleID)
}

func LoadTokenUsageAlertTokens(ctx context.Context, ids []int, after int) ([]Token, error) {
	var tokens []Token
	query := DB.WithContext(ctx).Model(&Token{}).Select("id,user_id,name,status,expired_time,remain_quota,used_quota,unlimited_quota,period_type,period_days,period_quota_limit,period_anchor_at,period_start_at,period_used_quota")
	if ids != nil {
		query = query.Where("id IN ?", ids)
	} else {
		query = query.Where("id > ?", after).Order("id")
	}
	err := query.Limit(200).Find(&tokens).Error
	return tokens, err
}

// The singleton budget lock serializes capacity changes and delivery claims on
// all supported databases. SMTP is never performed inside this transaction.
func ClaimTokenUsageAlert(ctx context.Context, userID, tokenID int, rule setting.TokenUsageAlertRule, now int64) (TokenUsageAlertDelivery, bool, error) {
	id := TokenUsageAlertID(userID, tokenID, rule.ID)
	var delivery TokenUsageAlertDelivery
	claimed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&TokenUsageAlertBudget{ID: 1}).Error; err != nil {
			return err
		}
		var budget TokenUsageAlertBudget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&budget, 1).Error; err != nil {
			return err
		}
		err := tx.Where("id = ?", id).First(&delivery).Error
		fresh := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !fresh {
			return err
		}
		if !fresh && delivery.CooldownUntil > now {
			if delivery.Status != "retryable" || delivery.NextAttemptAt > now || delivery.Attempts >= 3 {
				return nil
			}
		} else {
			delivery.Attempts = 0
		}
		if fresh && budget.Rows >= TokenUsageAlertCapacity {
			return errors.New("API Key usage alert capacity reached")
		}
		delivery.ID, delivery.UserID, delivery.TokenID, delivery.RuleID = id, userID, tokenID, rule.ID
		delivery.Status, delivery.UpdatedAt, delivery.NextAttemptAt = "sending", now, 0
		delivery.CooldownUntil = now + int64(rule.CooldownDays)*86400
		delivery.Attempts++
		if err := tx.Save(&delivery).Error; err != nil {
			return err
		}
		if fresh {
			if err := tx.Model(&budget).Update("entry_count", gorm.Expr("entry_count + 1")).Error; err != nil {
				return err
			}
		}
		claimed = true
		return nil
	})
	return delivery, claimed, err
}

func FinishTokenUsageAlert(ctx context.Context, delivery TokenUsageAlertDelivery, state string, now int64) (int64, error) {
	next := int64(0)
	if state == "retryable" && delivery.Attempts < 3 {
		next = now + int64(delivery.Attempts)*60
	} else if state != "sent" && state != "unknown" {
		state = "failed"
	}
	result := DB.WithContext(ctx).Model(&TokenUsageAlertDelivery{}).Where("id = ? AND status = ? AND updated_at = ? AND attempts = ?", delivery.ID, "sending", delivery.UpdatedAt, delivery.Attempts).Updates(map[string]any{"status": state, "next_attempt_at": next, "updated_at": now})
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, errors.New("API Key usage alert delivery changed")
	}
	return next, nil
}

func MaintainTokenUsageAlerts(ctx context.Context, now int64) ([]TokenUsageAlertDelivery, error) {
	var retries []TokenUsageAlertDelivery
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&TokenUsageAlertBudget{ID: 1}).Error; err != nil {
			return err
		}
		var budget TokenUsageAlertBudget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&budget, 1).Error; err != nil {
			return err
		}
		var expired []string
		if err := tx.Model(&TokenUsageAlertDelivery{}).Where("cooldown_until <= ? AND updated_at < ?", now, now-120).Order("cooldown_until").Limit(200).Pluck("id", &expired).Error; err != nil {
			return err
		}
		if len(expired) > 0 {
			result := tx.Where("id IN ?", expired).Delete(&TokenUsageAlertDelivery{})
			if result.Error != nil {
				return result.Error
			}
			if int(result.RowsAffected) > budget.Rows {
				return errors.New("API Key usage alert budget mismatch")
			}
			if err := tx.Model(&budget).Update("entry_count", gorm.Expr("entry_count - ?", result.RowsAffected)).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&TokenUsageAlertDelivery{}).Where("status = ? AND updated_at < ?", "sending", now-120).Update("status", "unknown").Error; err != nil {
			return err
		}
		return tx.Where("status = ? AND next_attempt_at <= ? AND attempts < 3", "retryable", now).Order("next_attempt_at").Limit(200).Find(&retries).Error
	})
	return retries, err
}
