package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLargeOriginalRefundLifecycle(t *testing.T) {
	for _, basis := range []string{"", RefundBasisApplicationBalance} {
		t.Run("basis="+basis, func(t *testing.T) {
			f := newRefundFinanceFixture(t)
			require.NoError(t, DB.AutoMigrate(&RefundAdjustment{}))
			t.Cleanup(func() { require.NoError(t, DB.Where("user_id = ?", f.user.Id).Delete(&RefundAdjustment{}).Error) })
			const requested = 3760000000
			const remaining = 3020667181
			seedRefundFinanceOrder(t, f.user.Id, requested+remaining, 1356134, 0, 1)
			require.NoError(t, DB.Model(&f.user).Update("quota", requested+remaining).Error)
			ticket, _, _, err := CreateRefundTicket(CreateRefundTicketParams{OriginalWorkflow: true, UserId: f.user.Id, Username: f.user.Username, RefundQuota: requested, PayeeType: "alipay", PayeeName: "test", PayeeAccount: "test", Contact: "test", RequestKey: uuid.NewString()})
			require.NoError(t, err)
			if basis != "" {
				plan, err := ManageRefundAdjustment(ticket.Id, f.finance.Id, nil, basis)
				require.NoError(t, err)
				require.True(t, plan.CanApply, plan.Blocker)
				_, err = ManageRefundAdjustment(ticket.Id, f.finance.Id, &RefundAdjustmentInput{CalculationMethod: "application_balance", Revision: plan.Revision})
				require.NoError(t, err)
			}
			data, err := SaveRefundInvoiceExport(ticket.Id, f.finance.Id)
			require.NoError(t, err)
			assert.Equal(t, int64(requested), data.PaidQuota)
			assert.Positive(t, data.CashCents)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
			require.NoError(t, err)
			var user User
			require.NoError(t, DB.First(&user, f.user.Id).Error)
			assert.Equal(t, remaining, user.Quota)
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
			require.Error(t, err)
		})
	}
}

func TestLargeRefundPaymentAllocation(t *testing.T) {
	orders := []TopUp{{Id: 1, UserId: 1, QuotaGranted: 350000000, PaymentTotalCents: 70000}, {Id: 2, UserId: 1, QuotaGranted: 3760000000, PaymentTotalCents: 800000, InvoiceTaxCents: 48000}}
	lots, err := projectRefundCreditLots(329332819, orders, nil)
	require.NoError(t, err)
	data, err := calculateOriginalRefundPayments(orders, lots, 3760000000, RefundGiftNone)
	require.NoError(t, err)
	assert.Equal(t, int64(799736), data.CashCents)
	assert.Equal(t, int64(3760000000), data.PaidQuota)
}

func TestLargeInvoiceTaxPaymentAndRefund(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 980811)
	require.NoError(t, DB.Model(user).Update("quota", 5000000000).Error)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, LegacyTopUpsEnabled: true, LegacyInvoiceBefore: 4102416000, Enabled: true, Mode: "vat", OrdinaryRateBps: 600}))
	t.Cleanup(func() {
		require.NoError(t, DB.Where("user_id = ?", user.Id).Delete(&InvoiceTaxCharge{}).Error)
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	})
	order := seedInvoiceTicketTopUp(t, user.Id, "large-tax-test", 100000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", order).Update("payment_provider", PaymentProviderEpay).Error)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{order}, "ordinary")
	require.NoError(t, err)
	assert.Equal(t, 3000000000, quote.Quota)
	params := createInvoiceTicketParams(user, []int{order})
	params.InvoiceType, params.TaxPaymentMethod, params.TaxQuote = "ordinary", "balance", quote.Token
	ticket, _, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	for range 2 {
		_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusRejected)
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 5000000000, user.Quota)
}

func TestLargeCommissionGrantAndClawback(t *testing.T) {
	f := newRefundFinanceFixture(t)
	oldRate, oldUnit := common.TopUpCommissionRate, common.QuotaPerUnit
	common.TopUpCommissionRate, common.QuotaPerUnit = 10, 500000
	t.Cleanup(func() { common.TopUpCommissionRate, common.QuotaPerUnit = oldRate, oldUnit })
	require.NoError(t, DB.Model(&f.user).Update("inviter_id", f.finance.Id).Error)
	order := seedRefundFinanceOrder(t, f.user.Id, 50000000000, 10000000, 0, 1)
	GrantTopUpCommission(order, false)
	GrantTopUpCommission(order, false)
	require.NoError(t, DB.First(&f.finance, f.finance.Id).Error)
	require.Equal(t, 5000000000, f.finance.AffQuota)
	require.NoError(t, DB.Create(&RefundOrderAllocation{RefundTicketID: 991, TopUpID: order.Id, UserID: f.user.Id, OriginalPaidCents: 10000000, RefundCents: 10000000, Status: "reserved"}).Error)
	refund := &TicketRefund{TicketId: 991, UserId: f.user.Id}
	quota, err := refundCommissionQuotaTx(DB, refund)
	require.NoError(t, err)
	assert.Equal(t, 5000000000, quota)
	require.NoError(t, DB.Model(&f.finance).Updates(map[string]interface{}{"aff_quota": 0, "quota": 0}).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		result, err := applyCommissionClawback(tx, refund, quota)
		if err == nil {
			assert.Equal(t, quota, result.ClawedQuota)
		}
		return err
	}))
	require.NoError(t, DB.First(&f.finance, f.finance.Id).Error)
	assert.Equal(t, -5000000000, f.finance.Quota)
	assert.Zero(t, f.finance.AffHistoryQuota)
}

func TestLargeReconciliationNoncashEvidence(t *testing.T) {
	quota, err := applyFinancePaymentEvidence(nil, FinanceReconciliationPlan{NoncashGrantQuota: 5000000000})
	require.NoError(t, err)
	assert.Equal(t, int64(5000000000), quota)
	_, err = applyFinancePaymentEvidence(nil, FinanceReconciliationPlan{NoncashGrantQuota: -1})
	require.Error(t, err)
}

func TestLargePaymentCreditAndBonusRollback(t *testing.T) {
	for _, provider := range []string{PaymentProviderStripe, PaymentProviderWaffo, PaymentProviderWaffoPancake} {
		for _, balance := range []int{0, math.MaxInt64} {
			t.Run(fmt.Sprintf("%s/%d", provider, balance), func(t *testing.T) {
				setupEpayRechargeTest(t)
				user, order := insertEpayRechargeFixture(t, "payment-bigint", provider, common.TopUpStatusPending, 10000, 10000, 0)
				require.NoError(t, DB.Model(user).Update("quota", balance).Error)
				require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"discount_code_id": 123, "discount_rate": 50}).Error)
				var err error
				switch provider {
				case PaymentProviderStripe:
					err = Recharge(order.TradeNo, "test-customer", "")
				case PaymentProviderWaffo:
					err = RechargeWaffo(order.TradeNo, "")
				case PaymentProviderWaffoPancake:
					err = RechargeWaffoPancake(order.TradeNo)
				}
				require.NoError(t, DB.First(user, user.Id).Error)
				require.NoError(t, DB.First(order, order.Id).Error)
				if balance == 0 {
					require.NoError(t, err)
					assert.Equal(t, 10000000000, user.Quota)
					assert.Equal(t, int64(5000000000), order.QuotaGranted)
				} else {
					require.Error(t, err)
					assert.Equal(t, balance, user.Quota)
					assert.Equal(t, common.TopUpStatusPending, order.Status)
					var n int64
					require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo+"_bonus").Count(&n).Error)
					assert.Zero(t, n)
				}
			})
		}
	}
}

func TestFinancialBalanceOverflowDoesNotMutateLedger(t *testing.T) {
	f := newRefundFinanceFixture(t)
	oldRate, oldUnit := common.TopUpCommissionRate, common.QuotaPerUnit
	common.TopUpCommissionRate, common.QuotaPerUnit = 10, 500000
	t.Cleanup(func() { common.TopUpCommissionRate, common.QuotaPerUnit = oldRate, oldUnit })
	require.NoError(t, DB.Model(&f.user).Update("inviter_id", f.finance.Id).Error)
	require.NoError(t, DB.Model(&f.finance).Updates(map[string]interface{}{"quota": math.MaxInt64, "aff_quota": math.MaxInt64, "aff_history": math.MaxInt64}).Error)
	order := seedRefundFinanceOrder(t, f.user.Id, 50000000000, 10000000, 0, 1)
	GrantTopUpCommission(order, false)
	var count int64
	require.NoError(t, DB.Model(&CommissionRecord{}).Where("top_up_id = ?", order.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.First(&f.finance, f.finance.Id).Error)
	assert.Equal(t, math.MaxInt64, f.finance.AffQuota)
	require.Error(t, f.finance.TransferAffQuotaToQuota(5000000000))
	require.NoError(t, DB.First(&f.finance, f.finance.Id).Error)
	assert.Equal(t, math.MaxInt64, f.finance.Quota)
	assert.Equal(t, math.MaxInt64, f.finance.AffQuota)
}

func TestSubscriptionFinancialQuotaRejectsOverflow(t *testing.T) {
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	quota, err := calcSubscriptionBalanceQuota(10000)
	require.NoError(t, err)
	assert.Equal(t, 5000000000, quota)
	for _, value := range []float64{math.MaxFloat64, math.NaN(), math.Inf(1)} {
		_, err = calcSubscriptionBalanceQuota(value)
		require.Error(t, err)
	}
}
