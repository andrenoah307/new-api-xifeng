package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceGroupRestrictionPreservesHistoricalEntitlements(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 981701)
	require.NoError(t, DB.Model(user).Update("quota", 100000000).Error)
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600, LegacyFreeOrdinaryEnabled: true, LegacyInvoiceBefore: 4102416000, InvoiceRestrictedGroups: []string{"default"}}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	legacyID := seedInvoiceTicketTopUp(t, user.Id, "restricted-legacy", 1000)
	typedID := seedInvoiceTicketTopUp(t, user.Id, "restricted-promised", 1000)
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", typedID).Update("invoice_type", "ordinary").Error)
	orders, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, typedID, orders[0].Id)
	for _, ids := range [][]int{{legacyID}, {legacyID, typedID}} {
		_, err := QuoteInvoiceTax(DB, user.Id, ids, "ordinary")
		assert.ErrorIs(t, err, ErrTicketForbidden)
		params := createInvoiceTicketParams(user, ids)
		params.InvoiceType = "ordinary"
		_, _, _, _, err = CreateInvoiceTicket(params)
		assert.ErrorIs(t, err, ErrTicketForbidden)
	}
	var count int64
	require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	var balance User
	require.NoError(t, DB.First(&balance, user.Id).Error)
	assert.Equal(t, 100000000, balance.Quota)
	_, _, err = CreateTicketWithMessage(CreateTicketParams{UserId: user.Id, Subject: "Invoice", Content: "Invoice", Type: TicketTypeInvoice})
	assert.ErrorIs(t, err, ErrTicketForbidden)
	_, _, err = CreateTicketWithMessage(CreateTicketParams{UserId: user.Id, Subject: "Support", Content: "Support", Type: TicketTypeGeneral})
	require.NoError(t, err)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{typedID}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(user, []int{typedID})
	params.InvoiceType, params.TaxQuote = "ordinary", quote.Token
	config.InvoiceRestrictedGroupsBlockTickets = true
	require.NoError(t, SaveInvoiceTaxConfig(config))
	orders, err = GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	assert.Empty(t, orders)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{typedID}, "ordinary")
	assert.ErrorIs(t, err, ErrTicketForbidden)
	_, _, _, _, err = CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	require.NoError(t, DB.First(&balance, user.Id).Error)
	assert.Equal(t, 100000000, balance.Quota)
	// The switch applies only to selected groups, even for existing invoice types.
	require.NoError(t, DB.Model(user).Update("group", "unrestricted").Error)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{typedID}, "ordinary")
	require.NoError(t, err)
	require.NoError(t, DB.Model(user).Update("group", "default").Error)
	config.InvoiceRestrictedGroupsBlockTickets = false
	require.NoError(t, SaveInvoiceTaxConfig(config))
	ticket, _, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	config.InvoiceRestrictedGroupsBlockTickets = true
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
	require.NoError(t, err)
	require.NoError(t, DB.Model(user).Update("group", "unrestricted").Error)
	_, err = QuoteInvoiceTax(DB, user.Id, []int{legacyID}, "ordinary")
	require.NoError(t, err)
}

func TestInvoiceGroupRestrictionBlocksCheckoutRetryButAcceptsPayment(t *testing.T) {
	setupEpayRechargeTest(t)
	oldPayment := *operation_setting.GetPaymentSetting()
	oldPrice, oldMin := operation_setting.Price, operation_setting.MinTopUp
	oldAddress, oldID, oldKey, oldMethods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://payment.example.test", "private", "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = oldPayment
		operation_setting.Price, operation_setting.MinTopUp = oldPrice, oldMin
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = oldAddress, oldID, oldKey, oldMethods
		require.NoError(t, DB.Where("key IN ?", []string{InvoiceTaxConfigKey, InvoiceEpayConfigKey}).Delete(&Option{}).Error)
	})
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	require.NoError(t, SaveInvoiceEpayConfig(InvoiceEpayConfig{Address: "https://payment.example.test", PartnerID: "invoice", Key: "secret", PaymentMethods: []string{"alipay"}}))
	user := insertInvoiceTicketUser(t, 981702)
	req := EpayCheckoutRequest{TotalCents: 10000, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "group-policy-request-001"}
	quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	req.Token = quote.Token
	order, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	config.InvoiceRestrictedGroups = []string{user.Group}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, err = QuoteEpayCheckout(DB, user.Id, req, false)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	_, err = CreateEpayCheckout(user.Id, req)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	req.RequestID = "group-policy-request-002"
	_, err = CreateEpayCheckout(user.Id, req)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	_, credit, err := CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
	require.NoError(t, err)
	assert.Equal(t, quote.Quota, credit)
	_, credit, err = CompleteEpayCheckout(order.TradeNo, "alipay", 10000)
	require.NoError(t, err)
	assert.Zero(t, credit)
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, quote.Quota, current.Quota)
	req.InvoiceType = "none"
	noneQuote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	assert.Zero(t, noneQuote.TaxCents)
	req.Token = noneQuote.Token
	noneOrder, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Equal(t, "none", noneOrder.InvoiceType)
}

func TestInvoiceGroupRestrictionPreservesExistingTaxPayment(t *testing.T) {
	truncateTables(t)
	user := insertInvoiceTicketUser(t, 981703)
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600, LegacyInvoiceBefore: 4102416000}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	orderID := seedInvoiceTicketTopUp(t, user.Id, "restricted-existing-tax", 1000)
	quote, err := QuoteInvoiceTax(DB, user.Id, []int{orderID}, "ordinary")
	require.NoError(t, err)
	params := createInvoiceTicketParams(user, []int{orderID})
	params.InvoiceType, params.TaxQuote, params.TaxPaymentMethod = "ordinary", quote.Token, "balance"
	ticket, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	require.Equal(t, "pending", invoice.TaxCharge.Status)
	require.NoError(t, DB.Model(invoice.TaxCharge).Update("payment_method", "alipay").Error)
	config.InvoiceRestrictedGroups = []string{user.Group}
	config.InvoiceRestrictedGroupsBlockTickets = true
	require.NoError(t, SaveInvoiceTaxConfig(config))
	require.NoError(t, SettleInvoiceTax(invoice.TaxCharge.TradeNo, "alipay", quote.FeeCents))
	_, _, _, err = UpdateInvoiceStatus(ticket.Id, user.Id, InvoiceStatusIssued)
	require.NoError(t, err)
}

func TestInvoiceGroupRestrictionPreventsNewAdminEligibility(t *testing.T) {
	owner, actor, order := invoiceAdjustmentFixture(t)
	config, err := GetInvoiceTaxConfig(DB)
	require.NoError(t, err)
	config.InvoiceRestrictedGroups = []string{owner.Group}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	_, err = ManageTopUpInvoiceType(actor.Id, TopUpInvoiceAdjustmentInput{ID: order.Id, InvoiceType: "ordinary", ExpectedInvoiceType: &order.InvoiceType}, false)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	var current TopUp
	require.NoError(t, DB.First(&current, order.Id).Error)
	assert.Equal(t, order.InvoiceType, current.InvoiceType)
	assert.Equal(t, order.QuotaGranted, current.QuotaGranted)
}
