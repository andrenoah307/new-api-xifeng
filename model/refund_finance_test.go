package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type refundFinanceFixture struct{ user, admin, root, finance User }

func newRefundFinanceFixture(t *testing.T) refundFinanceFixture {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundOrderAllocation{}).Error)
	require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceCorrection{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("1 = 1").Delete(&RefundOrderAllocation{}).Error)
		require.NoError(t, DB.Where("1 = 1").Delete(&RefundInvoiceCorrection{}).Error)
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300}))
	f := refundFinanceFixture{}
	for i, entry := range []struct {
		u    *User
		role int
	}{{&f.user, common.RoleCommonUser}, {&f.admin, common.RoleAdminUser}, {&f.root, common.RoleRootUser}, {&f.finance, common.RoleFinanceUser}} {
		*entry.u = User{Username: []string{"refund-owner", "refund-admin", "refund-root", "refund-finance"}[i], Role: entry.role, Status: common.UserStatusEnabled, AffCode: uuid.NewString()[:20]}
		require.NoError(t, DB.Create(entry.u).Error)
	}
	return f
}

func seedRefundFinanceOrder(t *testing.T, userID int, quota, paid, tax int64, completed int64) *TopUp {
	t.Helper()
	order := &TopUp{UserId: userID, TradeNo: uuid.NewString(), QuotaGranted: quota, Money: float64(paid) / 100, PaymentTotalCents: paid, InvoiceNetCents: paid - tax, InvoiceTaxCents: tax, CheckoutVersion: 1, InvoiceType: "ordinary", PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, CompleteTime: completed}
	require.NoError(t, DB.Create(order).Error)
	return order
}

func financeRefundParams(user User, quota int) CreateRefundTicketParams {
	quote, err := QuoteRefundFinance(user.Id, quota)
	params := CreateRefundTicketParams{UserId: user.Id, Username: user.Username, RefundQuota: quota, PayeeType: RefundPayeeTypeAlipay, PayeeName: "payee", PayeeAccount: "account", Contact: "contact", RequestKey: uuid.NewString()}
	if err == nil && quote != nil {
		params.FinanceQuote = quote.QuoteToken
	}
	return params
}

func recordHistoricalRefund(t *testing.T, userID, quota int) *RefundFinanceDetail {
	t.Helper()
	var detail *RefundFinanceDetail
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		detail, err = quoteRefundFinanceTx(tx, userID, quota)
		if err != nil {
			return err
		}
		ticket := Ticket{UserId: userID, Type: TicketTypeRefund, Status: TicketStatusResolved}
		if err = tx.Create(&ticket).Error; err != nil {
			return err
		}
		refund := TicketRefund{TicketId: ticket.Id, UserId: userID, RefundQuota: quota, FinanceVersion: 1, RefundCashCents: detail.RefundCashCents, RefundStatus: RefundStatusRefunded}
		if err = tx.Create(&refund).Error; err != nil {
			return err
		}
		for i := range detail.Allocations {
			detail.Allocations[i].RefundTicketID = ticket.Id
			detail.Allocations[i].Status = "committed"
		}
		if err = tx.Create(&detail.Allocations).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("quota", gorm.Expr("quota - ?", quota)).Error
	}))
	return detail
}
func TestRefundWaitingReservesWalletAndBlocksEveryApprovalMode(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 10000, 1060000, 60000, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 5000, "used_quota": 5000}).Error)
	quote, err := QuoteRefundFinance(f.user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, RefundSettlementWaiting, quote.SettlementStatus)
	assert.Empty(t, quote.Allocations)
	assert.Zero(t, quote.RefundCashCents)
	assert.Empty(t, quote.QuoteToken)
	params := financeRefundParams(f.user, 5000)
	ticket, refund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, 5000, refund.FrozenQuota)
	assert.Zero(t, refund.RefundCashCents)
	assert.Empty(t, refund.Finance.Allocations)
	assert.Empty(t, refund.Finance.InvoiceCorrections)
	require.NoError(t, DecreaseUserQuota(f.user.Id, 1000, true))
	for _, actor := range []int{f.admin.Id, f.root.Id} {
		for _, mode := range []string{"", RefundQuotaModeWriteOff, RefundQuotaModeSubtract, RefundQuotaModeOverride} {
			_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: actor, RefundStatus: RefundStatusRefunded, QuotaMode: mode, ActualRefundQuota: 1})
			require.ErrorIs(t, err, ErrRefundSettlementUnconfirmed)
		}
	}
	replay, _, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, ticket.Id, replay.Id)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, -1000, owner.Quota)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.NoError(t, err)
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 4000, owner.Quota)
}
func TestRefundFinanceIssuedInvoiceMathAndBigintWallet(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 5000000000, 1060000, 60000, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 2500000000, "used_quota": 2500000000}).Error)
	quote, err := quoteRefundFinanceTx(DB, f.user.Id, 2500000000)
	require.NoError(t, err)
	assert.Equal(t, int64(530000), quote.RefundCashCents)
	assert.Equal(t, int64(30000), quote.RefundTaxCents)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 2500000000, owner.Quota)
}
func TestRefundFinanceFIFOAndRoundingConserveCents(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 3, 101, 6, 100)
	second := seedRefundFinanceOrder(t, f.user.Id, 2, 226, 26, 200)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 5).Error)
	var cash, tax int64
	for _, expected := range []struct {
		id        int
		cash, tax int64
	}{{first.Id, 34, 2}, {first.Id, 33, 2}, {first.Id, 34, 2}, {second.Id, 113, 13}, {second.Id, 113, 13}} {
		detail := recordHistoricalRefund(t, f.user.Id, 1)
		require.Len(t, detail.Allocations, 1)
		assert.Equal(t, expected.id, detail.Allocations[0].TopUpID)
		assert.Equal(t, expected.cash, detail.RefundCashCents)
		assert.Equal(t, expected.tax, detail.RefundTaxCents)
		cash += detail.RefundCashCents
		tax += detail.RefundTaxCents
	}
	assert.Equal(t, int64(327), cash)
	assert.Equal(t, int64(32), tax)
}
func TestRefundFinanceHistoricalRemainingInvoice(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	recordHistoricalRefund(t, f.user.Id, 400)
	quote, err := QuoteInvoiceTax(DB, f.user.Id, []int{order.Id}, "ordinary")
	require.NoError(t, err)
	assert.Equal(t, int64(6360), quote.TotalCents)
	var original TopUp
	require.NoError(t, DB.First(&original, order.Id).Error)
	assert.Equal(t, int64(10600), original.PaymentTotalCents)
}
func TestRefundWaitingAlsoAppliesWhenInvoiceModuleDisabled(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	_, err := quoteRefundFinanceTx(DB, f.user.Id, 400)
	require.ErrorIs(t, err, ErrRefundFinanceReview)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Mode: "vat"}))
	params := financeRefundParams(f.user, 400)
	params.RequestKey = ""
	_, _, _, err = CreateRefundTicket(params)
	require.Error(t, err)
	params.RequestKey = uuid.NewString()
	_, refund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, 1, refund.FinanceVersion)
	assert.Equal(t, 400, refund.FrozenQuota)
}

func TestRefundWaitingReservesAvailableBalanceDuringPreconsume(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	// The request has preconsumed 200; UsedQuota has not been settled yet.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 800).Error)
	eligibility, err := GetRefundEligibility(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, eligibility.MaxRefundableQuota)
	assert.Equal(t, 800, eligibility.RequestableQuota)
	assert.Equal(t, []RefundExclusion{{Code: "unverified_balance", Quota: 800}}, eligibility.Reasons)
	quote, err := QuoteRefundFinance(f.user.Id, 800)
	require.NoError(t, err)
	assert.Equal(t, RefundSettlementWaiting, quote.SettlementStatus)
	ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, 800))
	require.NoError(t, err)
	assert.Equal(t, 800, refund.FrozenQuota)
	// Finishing the charge does not automatically prove that no other request is in flight.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("used_quota", 200).Error)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.ErrorIs(t, err, ErrRefundSettlementUnconfirmed)
	_, err = CloseUserTicket(ticket.Id, f.user.Id)
	require.NoError(t, err)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 800, owner.Quota)
}
func TestRefundFinanceFIFOConsumptionAcrossDifferentRates(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 100, 10600, 600, 100)
	second := seedRefundFinanceOrder(t, f.user.Id, 100, 11300, 1300, 200)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 120, "used_quota": 80}).Error)
	quote, err := quoteRefundFinanceTx(DB, f.user.Id, 50)
	require.NoError(t, err)
	require.Len(t, quote.Allocations, 2)
	assert.Equal(t, first.Id, quote.Allocations[0].TopUpID)
	assert.Equal(t, int64(80), quote.Allocations[0].UsedQuota)
	assert.Equal(t, int64(20), quote.Allocations[0].RefundQuota)
	assert.Equal(t, second.Id, quote.Allocations[1].TopUpID)
	assert.Zero(t, quote.Allocations[1].UsedQuota)
	assert.Equal(t, int64(30), quote.Allocations[1].RefundQuota)
	assert.Equal(t, int64(5510), quote.RefundCashCents)
	assert.Equal(t, int64(510), quote.RefundTaxCents)
}

func TestRefundWalletSnapshotsUseBigintAcrossSQLDialects(t *testing.T) {
	parsed, err := schema.Parse(&TicketRefund{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, name := range []string{"RefundQuota", "FrozenQuota", "UserQuotaSnapshot"} {
		field := parsed.LookUpField(name)
		require.NotNil(t, field)
		assert.Equal(t, "bigint", mysql.New(mysql.Config{}).DataTypeOf(field))
		assert.Equal(t, "bigint", postgres.New(postgres.Config{}).DataTypeOf(field))
	}
}

func TestRefundFinanceManualGiftCannotMaskConsumedPaidBalance(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	// A manual credit has replaced spent quota, but it is not refundable cash.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 1000, "used_quota": 100}).Error)
	_, err := quoteRefundFinanceTx(DB, f.user.Id, 400)
	require.ErrorIs(t, err, ErrRefundFinanceReview)
}

func TestRefundFinanceZeroCashRoundingCannotCreateTaxOnlyRefund(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 100000, 10600, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 100000).Error)
	first := recordHistoricalRefund(t, f.user.Id, 83)
	assert.Equal(t, int64(9), first.RefundCashCents)
	assert.Equal(t, int64(1), first.RefundTaxCents)
	quote, err := quoteRefundFinanceTx(DB, f.user.Id, 1)
	require.NoError(t, err)
	assert.Zero(t, quote.RefundCashCents)
	assert.Zero(t, quote.RefundTaxCents)
}
