package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundEligibilityExplainsCurrentWalletWithoutDoubleCounting(t *testing.T) {
	for _, test := range []struct {
		name                                           string
		wallet, used, paid, bonus, invite, frozen, tax int
		maximum                                        int
		reasons                                        []RefundExclusion
	}{
		{"cash-only", 1000, 0, 1000, 0, 0, 0, 0, 1000, nil},
		{"unspent-grants", 1300, 0, 1000, 200, 100, 0, 0, 1000, []RefundExclusion{{"gift_balance", 200}, {"invite_reward_balance", 100}}},
		{"possibly-spent-gift", 900, 300, 1000, 200, 0, 0, 0, 700, []RefundExclusion{{"unverified_balance", 200}}},
		{"partly-proven-gift", 1100, 100, 1000, 200, 0, 0, 0, 900, []RefundExclusion{{"gift_balance", 100}, {"unverified_balance", 100}}},
		{"partly-proven-reward", 1100, 100, 1000, 0, 200, 0, 0, 900, []RefundExclusion{{"invite_reward_balance", 100}, {"unverified_balance", 100}}},
		{"historical-grants-exceed-wallet", 50, 250, 0, 200, 100, 0, 0, 0, []RefundExclusion{{"unverified_balance", 50}}},
		{"manual-credit-masks-consumption", 1000, 100, 1000, 0, 0, 0, 0, 0, []RefundExclusion{{"unverified_balance", 1000}}},
		{"unbacked-balance", 200, 0, 0, 0, 0, 0, 0, 0, []RefundExclusion{{"unverified_balance", 200}}},
		{"frozen-not-in-wallet", 800, 0, 1000, 0, 0, 200, 0, 800, nil},
		{"balance-tax-spending", 1100, 0, 1000, 200, 0, 0, 100, 900, []RefundExclusion{{"gift_balance", 100}, {"unverified_balance", 100}}},
		{"bigint-balance", 6_000_000_000, 0, 6_000_000_000, 0, 0, 0, 0, 6_000_000_000, nil},
		{"exact-json-integer-limit", 9_007_199_254_740_991, 0, 9_007_199_254_740_991, 0, 0, 0, 0, 9_007_199_254_740_991, nil},
		{"exact-frozen-json-integer-limit", 0, 0, 9_007_199_254_740_991, 0, 0, 9_007_199_254_740_991, 0, 0, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			owner := User{Username: "eligibility", Quota: test.wallet, UsedQuota: test.used, AffHistoryQuota: test.invite + 10, AffQuota: 10}
			require.NoError(t, DB.Create(&owner).Error)
			for _, credit := range []struct {
				quota  int
				source string
			}{{test.paid, ""}, {test.bonus, "discount_bonus"}} {
				if credit.quota > 0 {
					require.NoError(t, DB.Create(&TopUp{UserId: owner.Id, TradeNo: uuid.NewString(), Status: common.TopUpStatusSuccess, QuotaGranted: int64(credit.quota), Source: credit.source}).Error)
				}
			}
			if test.frozen > 0 {
				require.NoError(t, DB.Create(&TicketRefund{TicketId: owner.Id, UserId: owner.Id, RefundQuota: test.frozen, FrozenQuota: test.frozen, RefundStatus: RefundStatusPending}).Error)
			}
			if test.tax > 0 {
				require.NoError(t, DB.Create(&InvoiceTaxCharge{UserId: owner.Id, TradeNo: uuid.NewString(), Status: "paid", PaymentMethod: "balance", Quota: test.tax}).Error)
				t.Cleanup(func() { require.NoError(t, DB.Where("user_id = ?", owner.Id).Delete(&InvoiceTaxCharge{}).Error) })
			}
			result, err := GetRefundEligibility(owner.Id)
			require.NoError(t, err)
			assert.Equal(t, test.wallet, result.WalletQuota)
			assert.Equal(t, test.maximum, result.MaxRefundableQuota)
			assert.GreaterOrEqual(t, result.RequestableQuota, result.MaxRefundableQuota)
			assert.LessOrEqual(t, result.RequestableQuota, result.WalletQuota)
			if test.name == "unspent-grants" {
				assert.Equal(t, 1000, result.RequestableQuota, "known gifts and referral rewards cannot be included in a waiting refund")
			}
			assert.Equal(t, test.frozen, result.FrozenQuota)
			assert.Equal(t, test.wallet-test.maximum, result.NonRefundableQuota)
			assert.Equal(t, append([]RefundExclusion{}, test.reasons...), result.Reasons)
			total := 0
			for _, reason := range result.Reasons {
				assert.Positive(t, reason.Quota)
				total += reason.Quota
			}
			assert.Equal(t, result.NonRefundableQuota, total)
			var unchanged User
			require.NoError(t, DB.First(&unchanged, owner.Id).Error)
			assert.Equal(t, test.wallet, unchanged.Quota)
		})
	}
}

func TestRefundEligibilityAggregateOverflowFailsClosed(t *testing.T) {
	truncateTables(t)
	owner := User{Username: "eligibility-overflow", Quota: 100}
	require.NoError(t, DB.Create(&owner).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, DB.Create(&TopUp{UserId: owner.Id, TradeNo: uuid.NewString(), Status: common.TopUpStatusSuccess, QuotaGranted: math.MaxInt64}).Error)
	}
	result, err := GetRefundEligibility(owner.Id)
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestRefundEligibilityRejectsUnsafeJSONIntegers(t *testing.T) {
	for _, field := range []string{"wallet", "frozen"} {
		t.Run(field, func(t *testing.T) {
			truncateTables(t)
			owner := User{Username: "unsafe-json-refund", Quota: 1}
			if field == "wallet" {
				owner.Quota = 9_007_199_254_740_992
			}
			require.NoError(t, DB.Create(&owner).Error)
			require.NoError(t, DB.Create(&TopUp{UserId: owner.Id, TradeNo: uuid.NewString(), Status: common.TopUpStatusSuccess, QuotaGranted: int64(owner.Quota)}).Error)
			if field == "frozen" {
				require.NoError(t, DB.Create(&TicketRefund{TicketId: owner.Id, UserId: owner.Id, RefundQuota: 1, FrozenQuota: 9_007_199_254_740_992, RefundStatus: RefundStatusPending}).Error)
			}
			result, err := GetRefundEligibility(owner.Id)
			require.ErrorIs(t, err, ErrRefundFinanceReview)
			assert.Nil(t, result)
		})
	}
}
