package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const maxRefundEligibilityJSONQuota int64 = 1<<53 - 1

type RefundExclusion struct {
	Code  string `json:"code"`
	Quota int    `json:"quota"`
}

type RefundEligibility struct {
	WalletQuota        int               `json:"wallet_quota"`
	MaxRefundableQuota int               `json:"max_refundable_quota"`
	RequestableQuota   int               `json:"requestable_quota"`
	NonRefundableQuota int               `json:"non_refundable_quota"`
	Reasons            []RefundExclusion `json:"reasons"`
	FrozenQuota        int               `json:"frozen_quota"`
}

func GetRefundEligibility(userID int) (*RefundEligibility, error) {
	var result *RefundEligibility
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = getRefundEligibilityTx(tx, userID)
		return err
	})
	return result, err
}

// Explain only amounts provably still present in the wallet. Historical grants
// are not a balance ledger: amounts possibly consumed remain unverified.
func getRefundEligibilityTx(tx *gorm.DB, userID int) (*RefundEligibility, error) {
	var owner User
	if err := tx.Select("id, quota, used_quota, aff_history, aff_quota").First(&owner, userID).Error; err != nil {
		return nil, err
	}
	if int64(owner.Quota) > maxRefundEligibilityJSONQuota {
		return nil, ErrRefundFinanceReview
	}
	maximum, err := getUserMaxRefundableQuotaTx(tx, userID)
	if err != nil {
		return nil, err
	}
	var credits struct {
		Paid  int64
		Bonus int64
	}
	if err := tx.Model(&TopUp{}).Where("user_id = ? AND status = ?", userID, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(CASE WHEN source = ? THEN 0 ELSE quota_granted END),0) AS paid, COALESCE(SUM(CASE WHEN source = ? THEN quota_granted ELSE 0 END),0) AS bonus", "discount_bonus", "discount_bonus").Scan(&credits).Error; err != nil {
		return nil, err
	}
	var refunds struct {
		Refunded int64
		Frozen   int64
	}
	if err := tx.Model(&TicketRefund{}).Where("user_id = ?", userID).
		Select("COALESCE(SUM(CASE WHEN refund_status = ? THEN refund_quota ELSE 0 END),0) AS refunded, COALESCE(SUM(CASE WHEN refund_status = ? THEN frozen_quota ELSE 0 END),0) AS frozen", RefundStatusRefunded, RefundStatusPending).Scan(&refunds).Error; err != nil {
		return nil, err
	}
	if refunds.Frozen > maxRefundEligibilityJSONQuota {
		return nil, ErrRefundFinanceReview
	}
	var balanceTax int64
	if err := tx.Model(&InvoiceTaxCharge{}).Where("user_id = ? AND status = ? AND payment_method = ?", userID, "paid", "balance").Select("COALESCE(SUM(quota),0)").Scan(&balanceTax).Error; err != nil {
		return nil, err
	}
	result := &RefundEligibility{WalletQuota: max(0, owner.Quota), Reasons: []RefundExclusion{}, FrozenQuota: int(max(int64(0), refunds.Frozen))}
	invitation := max(int64(0), int64(max(0, owner.AffHistoryQuota))-int64(max(0, owner.AffQuota)))
	known := decimal.NewFromInt(credits.Paid).Add(decimal.NewFromInt(credits.Bonus)).Add(decimal.NewFromInt(invitation)).Sub(decimal.NewFromInt(refunds.Refunded)).Sub(decimal.NewFromInt(refunds.Frozen))
	accounted := decimal.NewFromInt(int64(owner.Quota)).Add(decimal.NewFromInt(int64(owner.UsedQuota))).Add(decimal.NewFromInt(balanceTax))
	valid := owner.Quota >= 0 && owner.UsedQuota >= 0 && owner.AffQuota >= 0 && owner.AffHistoryQuota >= 0 && credits.Paid >= 0 && credits.Bonus >= 0 && refunds.Refunded >= 0 && refunds.Frozen >= 0 && balanceTax >= 0 && known.Equal(accounted)
	if valid {
		result.MaxRefundableQuota = min(result.WalletQuota, max(0, maximum))
	}
	result.NonRefundableQuota = result.WalletQuota - result.MaxRefundableQuota
	remaining := result.NonRefundableQuota
	if valid {
		// Each lower bound assumes every known expense could have consumed this
		// source. This deliberately avoids assigning uncertain grants twice.
		for _, source := range []struct {
			code  string
			total int64
		}{{"gift_balance", credits.Bonus}, {"invite_reward_balance", invitation}} {
			lower := max(int64(0), source.total-int64(owner.UsedQuota))
			lower = max(int64(0), lower-balanceTax)
			amount := min(remaining, int(lower))
			if amount > 0 {
				result.Reasons = append(result.Reasons, RefundExclusion{Code: source.code, Quota: amount})
				remaining -= amount
			}
		}
	}
	if remaining > 0 {
		result.Reasons = append(result.Reasons, RefundExclusion{Code: "unverified_balance", Quota: remaining})
	}
	// Unverified funds may be recorded for review, but never frozen or paid out.
	// Only proven noncash balances are excluded from the waiting application.
	result.RequestableQuota = result.MaxRefundableQuota + remaining
	return result, nil
}
