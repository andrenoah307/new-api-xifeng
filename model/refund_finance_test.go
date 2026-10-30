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
		*entry.u = User{Username: []string{"refund-owner", "refund-admin", "refund-root", "refund-finance"}[i], Role: entry.role, Status: common.UserStatusEnabled, AffCode: uuid.NewString()}
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

func TestRefundFinanceIssuedInvoiceCorrectionAndBigintWallet(t *testing.T) {
	f := newRefundFinanceFixture(t)
	const originalQuota int64 = 5_000_000_000
	const refundQuota = 2_500_000_000
	order := seedRefundFinanceOrder(t, f.user.Id, originalQuota, 1060000, 60000, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": refundQuota, "used_quota": originalQuota - refundQuota}).Error)
	invoiceTicket := Ticket{UserId: f.user.Id, Username: f.user.Username, Type: TicketTypeInvoice, Status: TicketStatusResolved}
	require.NoError(t, DB.Create(&invoiceTicket).Error)
	ids, err := common.Marshal([]int{order.Id})
	require.NoError(t, err)
	require.NoError(t, DB.Create(&TicketInvoice{TicketId: invoiceTicket.Id, UserId: f.user.Id, TopUpOrderIds: string(ids), TotalMoney: 10600, InvoiceStatus: InvoiceStatusIssued, InvoiceType: "ordinary"}).Error)
	quote, err := QuoteRefundFinance(f.user.Id, refundQuota)
	require.NoError(t, err)
	assert.Equal(t, int64(530000), quote.RefundCashCents)
	assert.Equal(t, int64(30000), quote.RefundTaxCents)
	params := financeRefundParams(f.user, refundQuota)
	ticket, refund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, refundQuota, refund.RefundQuota)
	assert.Equal(t, refundQuota, refund.FrozenQuota)
	assert.Equal(t, refundQuota, refund.UserQuotaSnapshot)
	require.NotNil(t, refund.Finance)
	require.Len(t, refund.Finance.InvoiceCorrections, 1)
	correction := refund.Finance.InvoiceCorrections[0]
	assert.Equal(t, int64(1060000), correction.OriginalCents)
	assert.Equal(t, int64(530000), correction.ReissueCents)
	again, againRefund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Equal(t, ticket.Id, again.Id)
	assert.Equal(t, refund.Id, againRefund.Id)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Zero(t, owner.Quota)
	approve := UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded}
	_, _, _, _, err = UpdateRefundStatus(approve)
	require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
	_, err = StartRefundInvoiceCorrection(correction.Id, f.finance.Id)
	require.NoError(t, err)
	_, err = CompleteRefundInvoiceCorrection(correction.Id, f.finance.Id, "red-1", "blue-2", "verified")
	require.NoError(t, err)
	approve.AdminId = f.finance.Id
	_, _, _, _, err = UpdateRefundStatus(approve)
	require.ErrorIs(t, err, ErrTicketForbidden)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
	_, err = CloseUserTicket(ticket.Id, f.user.Id)
	require.ErrorIs(t, err, ErrRefundFinanceCorrectionPending)
	approve.AdminId = f.root.Id
	settled, _, _, _, err := UpdateRefundStatus(approve)
	require.NoError(t, err)
	assert.Equal(t, RefundStatusRefunded, settled.RefundStatus)
	require.Len(t, settled.Finance.Allocations, 1)
	assert.Equal(t, "committed", settled.Finance.Allocations[0].Status)
}

func TestRefundFinanceFIFOAndRoundingConserveCents(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 3, 101, 6, 100)
	second := seedRefundFinanceOrder(t, f.user.Id, 2, 226, 26, 200)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 5).Error)
	var cash, tax int64
	for i, expected := range []struct {
		orderID   int
		cash, tax int64
	}{{first.Id, 34, 2}, {first.Id, 33, 2}, {first.Id, 34, 2}, {second.Id, 113, 13}, {second.Id, 113, 13}} {
		ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, 1))
		require.NoError(t, err, "refund %d", i)
		require.Len(t, refund.Finance.Allocations, 1)
		assert.Equal(t, expected.orderID, refund.Finance.Allocations[0].TopUpID)
		assert.Equal(t, expected.cash, refund.RefundCashCents)
		assert.Equal(t, expected.tax, refund.Finance.RefundTaxCents)
		cash += refund.RefundCashCents
		tax += refund.Finance.RefundTaxCents
		_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
		require.NoError(t, err)
	}
	assert.Equal(t, int64(327), cash)
	assert.Equal(t, int64(32), tax)
}

func TestRefundFinanceReservationAndRemainingInvoice(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	ticket, _, _, err := CreateRefundTicket(financeRefundParams(f.user, 400))
	require.NoError(t, err)
	_, err = QuoteInvoiceTax(DB, f.user.Id, []int{order.Id}, "ordinary")
	require.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	_, err = QuoteRefundFinance(f.user.Id, 100)
	require.ErrorIs(t, err, ErrRefundFinancePending)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	quote, err := QuoteInvoiceTax(DB, f.user.Id, []int{order.Id}, "ordinary")
	require.NoError(t, err)
	assert.Equal(t, int64(6360), quote.TotalCents)
	var unchanged TopUp
	require.NoError(t, DB.First(&unchanged, order.Id).Error)
	assert.Equal(t, int64(10600), unchanged.PaymentTotalCents)
}

func TestRefundFinanceLegacyOrdersRequireReviewAndDisabledModulePreservesLegacy(t *testing.T) {
	f := newRefundFinanceFixture(t)
	order := seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	_, err := QuoteRefundFinance(f.user.Id, 400)
	require.ErrorIs(t, err, ErrRefundFinanceReview)
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: false, Mode: "vat"}))
	quote, err := QuoteRefundFinance(f.user.Id, 400)
	require.NoError(t, err)
	assert.Nil(t, quote)
	params := financeRefundParams(f.user, 400)
	params.RequestKey = ""
	_, refund, _, err := CreateRefundTicket(params)
	require.NoError(t, err)
	assert.Zero(t, refund.FinanceVersion)
	assert.Nil(t, refund.Finance)
}

func TestRefundFinanceFIFOConsumptionAcrossDifferentRates(t *testing.T) {
	f := newRefundFinanceFixture(t)
	first := seedRefundFinanceOrder(t, f.user.Id, 100, 10600, 600, 100)
	second := seedRefundFinanceOrder(t, f.user.Id, 100, 11300, 1300, 200)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 120, "used_quota": 80}).Error)
	quote, err := QuoteRefundFinance(f.user.Id, 50)
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

func TestRefundFinanceStaleQuoteDoesNotFreezeOrCreateTicket(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 1000).Error)
	params := financeRefundParams(f.user, 400)
	require.NotEmpty(t, params.FinanceQuote)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 900, "used_quota": 100}).Error)
	_, _, _, err := CreateRefundTicket(params)
	require.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 900, owner.Quota)
	var count int64
	require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ?", f.user.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&RefundOrderAllocation{}).Where("user_id = ?", f.user.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRefundFinanceManualGiftCannotMaskConsumedPaidBalance(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 1000, 10600, 600, 100)
	// A manual credit has replaced spent quota, but it is not refundable cash.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"quota": 1000, "used_quota": 100}).Error)
	_, err := QuoteRefundFinance(f.user.Id, 400)
	require.ErrorIs(t, err, ErrRefundFinanceReview)
}

func TestRefundFinanceZeroCashRoundingCannotCreateTaxOnlyRefund(t *testing.T) {
	f := newRefundFinanceFixture(t)
	seedRefundFinanceOrder(t, f.user.Id, 100000, 10600, 600, 100)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", 100000).Error)
	ticket, refund, _, err := CreateRefundTicket(financeRefundParams(f.user, 83))
	require.NoError(t, err)
	assert.Equal(t, int64(9), refund.RefundCashCents)
	assert.Equal(t, int64(1), refund.Finance.RefundTaxCents)
	_, _, _, _, err = UpdateRefundStatus(UpdateRefundStatusParams{TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRefunded})
	require.NoError(t, err)
	quote, err := QuoteRefundFinance(f.user.Id, 1)
	require.NoError(t, err)
	assert.Zero(t, quote.RefundCashCents)
	assert.Zero(t, quote.RefundTaxCents)
}
