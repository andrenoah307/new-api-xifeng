package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundGiftPolicyRejectsInvalidConfig(t *testing.T) {
	newRefundFinanceFixture(t)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.RefundGiftRecoveryPolicy = "unknown"
	require.ErrorIs(t, SaveInvoiceTaxConfig(config), ErrInvoiceTaxInvalid)
	current, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, RefundGiftPriorExpiry, current.RefundGiftRecoveryPolicy)
}

func TestOriginalRefundCashAndGiftRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		orders                         int
		used, request, gift, remaining int
		mode                           string
		policy                         string
	}{
		{"one_order", 1, 45000, 45000, 5000, 5000, RefundQuotaModeWriteOff, ""},
		{"two_orders", 2, 135000, 45000, 15000, 5000, RefundQuotaModeWriteOff, ""},
		{"override", 1, 45000, 45000, 5000, 5000, RefundQuotaModeOverride, ""},
		{"partial", 1, 45000, 22500, 2500, 30000, RefundQuotaModeWriteOff, ""},
		{"proportional", 2, 135000, 45000, 5000, 15000, RefundQuotaModeWriteOff, RefundGiftProportional},
		{"none", 2, 135000, 45000, 0, 20000, RefundQuotaModeWriteOff, RefundGiftNone},
		{"cross_orders_expiry", 2, 45000, 135000, 20000, 0, RefundQuotaModeWriteOff, RefundGiftPriorExpiry},
		{"cross_orders_proportional", 2, 45000, 135000, 15000, 5000, RefundQuotaModeWriteOff, RefundGiftProportional},
		{"cross_orders_none", 2, 45000, 135000, 0, 20000, RefundQuotaModeWriteOff, RefundGiftNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			require.NoError(t, DB.AutoMigrate(&RefundInvoiceExport{}))
			require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error)
			t.Cleanup(func() { require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceExport{}).Error) })
			for i := 0; i < tc.orders; i++ {
				order := seedRefundFinanceOrder(t, f.user.Id, 90000, 9000, 0, int64(i+1))
				seedRefundGift(t, order, 10000)
			}
			require.NoError(t, DB.Model(&f.user).Updates(map[string]interface{}{"quota": tc.orders*100000 - tc.used, "used_quota": tc.used}).Error)
			params := CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: tc.request, PayeeType: "alipay", PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
			ticket, _, _, err := CreateRefundTicket(params)
			require.NoError(t, err)
			plan, err := SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
			require.NoError(t, err)
			config, err := GetInvoiceTaxConfig(DB)
			require.NoError(t, err)
			assert.Equal(t, RefundGiftPriorExpiry, config.RefundGiftRecoveryPolicy)
			if tc.policy != "" {
				config.RefundGiftRecoveryPolicy = tc.policy
				require.NoError(t, SaveInvoiceTaxConfig(config))
				plan, err = SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
				require.NoError(t, err)
			}
			assert.Equal(t, int64(tc.request/10), plan.CashCents)
			assert.Equal(t, int64(tc.gift), plan.RefundGiftQuota)
			refund, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded, QuotaMode: tc.mode, ActualRefundQuota: tc.remaining})
			require.NoError(t, err)
			assert.Equal(t, int64(tc.gift), refund.RefundGiftQuota)
			assert.Equal(t, int64(tc.request/10), refund.RefundCashCents)
			assert.Equal(t, tc.request, refund.RefundQuota)
			var owner User
			require.NoError(t, DB.First(&owner, f.user.Id).Error)
			assert.Equal(t, tc.remaining, owner.Quota)
			var recovered int64
			require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("refund_ticket_id = ? AND status = ?", ticket.Id, "committed").Select("COALESCE(SUM(refund_gift_quota), 0)").Scan(&recovered).Error)
			assert.Equal(t, int64(tc.gift), recovered)
			if tc.name != "partial" {
				completed, err := SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
				require.NoError(t, err)
				config.RefundGiftRecoveryPolicy = RefundGiftNone
				if tc.policy == RefundGiftNone {
					config.RefundGiftRecoveryPolicy = RefundGiftPriorExpiry
				}
				require.NoError(t, SaveInvoiceTaxConfig(config))
				unchanged, err := SaveRefundInvoiceExport(ticket.Id, f.admin.Id)
				require.NoError(t, err)
				assert.Equal(t, completed, unchanged)
				require.NoError(t, DB.First(&owner, f.user.Id).Error)
				assert.Equal(t, tc.remaining, owner.Quota)
			}
			if tc.name == "partial" {
				eligibility, err := OriginalRefundEligibility(f.user.Id)
				require.NoError(t, err)
				assert.Equal(t, 22500, eligibility.MaxRefundableQuota)
				params.RequestKey = uuid.NewString()
				next, _, _, err := CreateRefundTicket(params)
				require.NoError(t, err)
				refund, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: next.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
				require.NoError(t, err)
				assert.Equal(t, int64(2500), refund.RefundGiftQuota)
				require.NoError(t, DB.First(&owner, f.user.Id).Error)
				assert.Equal(t, 5000, owner.Quota)
			}
		})
	}
}
