package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func seedRefundGift(t *testing.T, order *TopUp, quota int64) {
	t.Helper()
	require.NoError(t, DB.Create(&TopUp{UserId: order.UserId, TradeNo: order.TradeNo + "_bonus", DiscountCodeId: order.DiscountCodeId, Source: "discount_bonus", QuotaGranted: quota, Status: common.TopUpStatusSuccess, CompleteTime: order.CompleteTime}).Error)
}

func TestRefundGiftUpgradePreservesExistingAllocations(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 45).Error)
	ticket := Ticket{UserId: f.user.Id, Type: TicketTypeRefund, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	refund := TicketRefund{TicketId: ticket.Id, UserId: f.user.Id, RefundQuota: 45, FrozenQuota: 45, FinanceVersion: 1, RefundCashCents: 4500, SettlementVerified: true, RefundStatus: RefundStatusPending}
	require.NoError(t, DB.Create(&refund).Error)
	allocation := RefundOrderAllocation{RefundTicketID: ticket.Id, TopUpID: order.Id, UserID: f.user.Id, OriginalQuota: 90, OriginalPaidCents: 9000, RefundQuota: 45, RefundCents: 4500, Status: "reserved"}
	require.NoError(t, DB.Create(&allocation).Error)
	require.NoError(t, DB.Migrator().DropColumn(&TicketRefund{}, "RefundGiftQuota"))
	for _, field := range []string{"GiftPolicyVersion", "GiftRecoveryReason", "OriginalGiftQuota", "RefundGiftQuota", "RefundGiftValueCents", "RemainingPaidValueCents", "RemainingGiftValueCents"} {
		require.NoError(t, DB.Migrator().DropColumn(&RefundOrderAllocation{}, field))
	}
	require.NoError(t, DB.AutoMigrate(&TicketRefund{}, &RefundOrderAllocation{}))
	require.NoError(t, DB.AutoMigrate(&TicketRefund{}, &RefundOrderAllocation{}))
	// Production upgrades restart the process; discard pgx connections holding
	// prepared SELECT * plans from the deliberately simulated old schema.
	if common.MainDatabaseType() == common.DatabaseTypePostgreSQL {
		connection, err := DB.DB()
		require.NoError(t, err)
		connection.SetMaxIdleConns(0)
		t.Cleanup(func() { connection.SetMaxIdleConns(2) })
	}
	require.NoError(t, DB.First(&refund, refund.Id).Error)
	require.NoError(t, DB.First(&allocation, allocation.ID).Error)
	assert.Zero(t, refund.RefundGiftQuota)
	assert.Zero(t, allocation.GiftPolicyVersion)
	assert.Zero(t, allocation.RefundGiftQuota)
	assert.Equal(t, int64(4500), allocation.RefundCents)
	assert.True(t, refund.hasVerifiedRefundHold())
	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 90, owner.Quota)
}

func TestRefundCreditLotCashFirstAndCancellation(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 100)
	seedRefundGift(t, order, 10)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 55, "used_quota": 45}).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 45))
	require.NoError(t, err)
	quote, err := QuoteRefundFinance(f.user.Id, 45)
	require.NoError(t, err)
	assert.Equal(t, int64(4500), quote.RefundCashCents)
	assert.Equal(t, int64(5), quote.RefundGiftQuota)
	assert.Equal(t, int64(500), quote.RefundGiftValueCents)
	confirmed, err := ConfirmRefundFinance(f.user.Id, ticket.Id, 45, quote.QuoteToken)
	require.NoError(t, err)
	assert.Equal(t, 50, confirmed.FrozenQuota)
	assert.Equal(t, int64(5), confirmed.RefundGiftQuota)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "0", report.Difference)
	var user User
	require.NoError(t, DB.First(&user, f.user.Id).Error)
	assert.Equal(t, 5, user.Quota)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	require.NoError(t, DB.First(&user, f.user.Id).Error)
	assert.Equal(t, 55, user.Quota)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.ErrorIs(t, err, ErrTicketRefundNotPending)
}

func TestRefundCreditLotAllPaidBeforeAnyGift(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 100)
	seedRefundGift(t, first, 10)
	second := seedRefundFinanceOrder(t, f.user.Id, 100, 10000, 600, 200)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 105, "used_quota": 95}).Error)
	maximum, err := GetUserMaxRefundableQuota(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 95, maximum)
	quote, err := quoteRefundFinanceTx(DB, f.user.Id, 95)
	require.NoError(t, err)
	require.Len(t, quote.Allocations, 2)
	assert.Equal(t, first.Id, quote.Allocations[0].TopUpID)
	assert.Zero(t, quote.Allocations[0].RefundCents)
	assert.Equal(t, "prior_order_expired", quote.Allocations[0].GiftRecoveryReason)
	assert.Equal(t, second.Id, quote.Allocations[1].TopUpID)
	assert.Equal(t, int64(5), quote.Allocations[1].UsedQuota)
	assert.Equal(t, int64(9500), quote.RefundCashCents)
	assert.Equal(t, int64(10), quote.RefundGiftQuota)
}

func TestRefundCreditLotTwoDiscountOrdersExpireEarlierGift(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 100)
	seedRefundGift(t, first, 10)
	second := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 200)
	seedRefundGift(t, second, 10)
	invoiceTicket := Ticket{UserId: f.user.Id, Type: TicketTypeInvoice, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&invoiceTicket).Error)
	ids, err := common.Marshal([]int{first.Id})
	require.NoError(t, err)
	require.NoError(t, DB.Create(&TicketInvoice{TicketId: invoiceTicket.Id, UserId: f.user.Id, TopUpOrderIds: string(ids), TotalMoney: 90, InvoiceStatus: InvoiceStatusIssued, InvoiceType: "ordinary"}).Error)
	require.NoError(t, DB.Create(&[]CommissionRecord{
		{UserId: f.user.Id, InviterId: f.admin.Id, TopUpId: first.Id, TopUpMoney: 90, CommissionQuota: 100, Type: CommissionTypeTopUp},
		{UserId: f.user.Id, InviterId: f.admin.Id, TopUpId: second.Id, TopUpMoney: 90, CommissionQuota: 200, Type: CommissionTypeTopUp},
	}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 65, "used_quota": 135}).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 45))
	require.NoError(t, err)
	quote, err := QuoteRefundFinance(f.user.Id, 45)
	require.NoError(t, err)
	assert.Equal(t, int64(4500), quote.RefundCashCents)
	assert.Equal(t, int64(15), quote.RefundGiftQuota)
	require.Len(t, quote.Allocations, 2)
	assert.Equal(t, int64(10), quote.Allocations[0].RefundGiftQuota)
	assert.Zero(t, quote.Allocations[0].RefundQuota)
	assert.Equal(t, int64(5), quote.Allocations[1].RefundGiftQuota)
	confirmed, err := ConfirmRefundFinance(f.user.Id, ticket.Id, 45, quote.QuoteToken)
	require.NoError(t, err)
	assert.Equal(t, 60, confirmed.FrozenQuota)
	assert.Empty(t, confirmed.Finance.InvoiceCorrections)
	clawback, err := refundCommissionQuotaTx(DB, confirmed)
	require.NoError(t, err)
	assert.Equal(t, 100, clawback)
	attempt, err := StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
	require.NoError(t, err)
	_, err = CompleteRefundPayout(attempt.Id, f.admin.Id, uuid.NewString(), "Verified bank receipt", true)
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 5, owner.Quota)
	maximum, err := GetUserMaxRefundableQuota(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, maximum)
	report, err := GetRefundReconciliationReport(ticket.Id)
	require.NoError(t, err)
	assert.Equal(t, "0", report.Difference)
	assert.Empty(t, report.Blockers)
	third := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 300)
	seedRefundGift(t, third, 10)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 105).Error)
	later, err := quoteRefundFinanceTx(DB, f.user.Id, 45)
	require.NoError(t, err)
	assert.Equal(t, int64(4500), later.RefundCashCents)
	assert.Equal(t, int64(10), later.RefundGiftQuota)
	require.Len(t, later.Allocations, 2)
	assert.Equal(t, second.Id, later.Allocations[0].TopUpID)
	assert.Equal(t, int64(5), later.Allocations[0].RefundGiftQuota)
}

func TestRefundCreditLotPartialPayoutConservesGift(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 3, 100, 6, 100)
	seedRefundGift(t, first, 2)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 5).Error)
	var cash, gifts, fees int64
	for _, expectedGift := range []int64{1, 0, 1} {
		ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 1))
		require.NoError(t, err)
		quote, err := QuoteRefundFinance(f.user.Id, 1)
		require.NoError(t, err)
		assert.Equal(t, expectedGift, quote.RefundGiftQuota)
		_, err = ConfirmRefundFinance(f.user.Id, ticket.Id, 1, quote.QuoteToken)
		require.NoError(t, err)
		attempt, err := StartRefundPayout(ticket.Id, f.admin.Id, uuid.NewString())
		require.NoError(t, err)
		_, err = CompleteRefundPayout(attempt.Id, f.admin.Id, uuid.NewString(), "Verified bank receipt", true)
		require.NoError(t, err)
		cash += quote.RefundCashCents
		gifts += quote.RefundGiftQuota
		fees += quote.RefundTaxCents
		report, err := GetRefundReconciliationReport(ticket.Id)
		require.NoError(t, err)
		assert.Equal(t, "0", report.Difference)
	}
	assert.Equal(t, int64(100), cash)
	assert.Equal(t, int64(2), gifts)
	assert.Equal(t, int64(6), fees)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Zero(t, owner.Quota)
}

func TestRefundCreditLotAlreadyConsumedGiftNotCash(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 90, 9000, 0, 100)
	seedRefundGift(t, first, 10)
	for _, used := range []int{90, 95} {
		require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 100 - used, "used_quota": used}).Error)
		maximum, err := GetUserMaxRefundableQuota(f.user.Id)
		require.NoError(t, err)
		assert.Zero(t, maximum)
		eligibility, err := GetRefundEligibility(f.user.Id)
		require.NoError(t, err)
		assert.Zero(t, eligibility.RequestableQuota)
		assert.Equal(t, []RefundExclusion{{Code: "gift_balance", Quota: 100 - used}}, eligibility.Reasons)
		_, err = quoteRefundFinanceTx(DB, f.user.Id, 1)
		require.ErrorIs(t, err, ErrTicketRefundQuotaExceed)
		var owner User
		require.NoError(t, DB.First(&owner, f.user.Id).Error)
		assert.Equal(t, 100-used, owner.Quota)
	}
}
