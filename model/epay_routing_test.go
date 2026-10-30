package model

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEpayRoutingTest(t *testing.T) InvoiceEpayConfig {
	t.Helper()
	setupEpayRechargeTest(t)
	oldPayment := *operation_setting.GetPaymentSetting()
	oldPrice, oldMin, oldMethods := operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods
	oldAddress, oldID, oldKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://private.example.test", "private-merchant", "private-key"
	keys := []string{InvoiceTaxConfigKey, InvoiceEpayConfigKey, "EpayRouteCursoralipaySecret", "EpayRouteCursorwxpaySecret"}
	require.NoError(t, DB.Where("key IN ?", keys).Delete(&Option{}).Error)
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting() = oldPayment
		operation_setting.Price, operation_setting.MinTopUp, operation_setting.PayMethods = oldPrice, oldMin, oldMethods
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = oldAddress, oldID, oldKey
		require.NoError(t, DB.Where("key IN ?", keys).Delete(&Option{}).Error)
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300}))
	config := InvoiceEpayConfig{Strategy: "round_robin", PrivateBelowCents: 50000, PrivateGatewayName: "Gateway P", Gateways: []InvoiceEpayGateway{
		{ID: "corporate-a", Name: "Gateway A", AccountName: "Issuer A", Address: "https://a.example.test", PartnerID: "a", Key: "key-a", PaymentMethods: []string{"alipay", "wxpay"}},
		{ID: "corporate-b", Name: "Gateway B", AccountName: "Issuer B", Address: "https://b.example.test", PartnerID: "b", Key: "key-b", PaymentMethods: []string{"alipay"}},
	}}
	require.NoError(t, SaveInvoiceEpayConfig(config))
	return config
}

func TestEpayRoutingAmountBoundaryRetainsInvoiceTax(t *testing.T) {
	setupEpayRoutingTest(t)
	user := insertInvoiceTicketUser(t, 981101)
	for _, test := range []struct {
		amount                 int64
		invoice, method, route string
		tax                    bool
	}{
		{49999, "ordinary", "alipay", "private", true},
		{50000, "ordinary", "wxpay", "corporate-a", true},
		{50001, "special", "wxpay", "corporate-a", true},
		{50000, "none", "alipay", "private", false},
	} {
		req := EpayCheckoutRequest{TotalCents: test.amount, InvoiceType: test.invoice, PaymentMethod: test.method}
		quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
		require.NoError(t, err)
		require.Len(t, quote.Routes, 1)
		assert.Equal(t, test.route, quote.Routes[0].GatewayID)
		if test.route == "private" {
			assert.Empty(t, quote.Routes[0].AccountName)
			assert.Equal(t, "Gateway P", quote.Routes[0].GatewayName)
		}
		assert.Equal(t, test.tax, quote.TaxCents > 0)
		assert.Equal(t, test.amount, quote.NetCents+quote.TaxCents)
		raw, err := common.Marshal(quote)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "Issuer")
		assert.NotContains(t, string(raw), "gateway_id")
		assert.NotContains(t, string(raw), "key-a")
	}
	methods, err := GetEpayCheckoutMethods(DB, "ordinary", 49999)
	require.NoError(t, err)
	assert.Equal(t, []string{"alipay"}, methods)
	methods, err = GetEpayCheckoutMethods(DB, "ordinary", 50000)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alipay", "wxpay"}, methods)
}

func TestEpayRoutingRoundRobinIdempotencyAndRollback(t *testing.T) {
	setupEpayRoutingTest(t)
	user := insertInvoiceTicketUser(t, 981102)
	req := EpayCheckoutRequest{TotalCents: 50000, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "routing-request-first"}
	quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	req.Token = quote.Token
	first, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Contains(t, first.EpayCredentials, `"gateway_id":"corporate-a"`)
	duplicate, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Equal(t, first.Id, duplicate.Id)
	req.RequestID = "routing-request-second"
	forced := errors.New("forced order write failure")
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("routing_fail_order", func(tx *gorm.DB) {
		if tx.Statement.Table == "top_ups" {
			tx.AddError(forced)
		}
	}))
	_, err = CreateEpayCheckout(user.Id, req)
	require.ErrorIs(t, err, forced)
	require.NoError(t, DB.Callback().Create().Remove("routing_fail_order"))
	second, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Contains(t, second.EpayCredentials, `"gateway_id":"corporate-b"`)
	req.RequestID = "routing-request-third"
	third, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Contains(t, third.EpayCredentials, `"gateway_id":"corporate-a"`)
}

func TestEpayRoutingConcurrentOrdersAndSnapshotExport(t *testing.T) {
	config := setupEpayRoutingTest(t)
	u1, u2 := insertInvoiceTicketUser(t, 981103), insertInvoiceTicketUser(t, 981104)
	req := EpayCheckoutRequest{TotalCents: 50000, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "routing-concurrent-request"}
	q1, err := QuoteEpayCheckout(DB, u1.Id, req, false)
	require.NoError(t, err)
	q2, err := QuoteEpayCheckout(DB, u2.Id, req, false)
	require.NoError(t, err)
	type result struct {
		order *TopUp
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, input := range []struct {
		user  int
		token string
	}{{u1.Id, q1.Token}, {u2.Id, q2.Token}} {
		wg.Add(1)
		go func(user int, token string) {
			defer wg.Done()
			request := req
			request.Token = token
			order, err := CreateEpayCheckout(user, request)
			results <- result{order, err}
		}(input.user, input.token)
	}
	wg.Wait()
	close(results)
	var orders []*TopUp
	var routes []string
	owners := map[int]int{}
	for result := range results {
		require.NoError(t, result.err)
		orders = append(orders, result.order)
		owners[result.order.Id] = result.order.UserId
		var snapshot InvoiceEpayConfig
		require.NoError(t, common.UnmarshalJsonStr(result.order.EpayCredentials, &snapshot))
		routes = append(routes, snapshot.GatewayID)
	}
	assert.ElementsMatch(t, []string{"corporate-a", "corporate-b"}, routes)
	smallRequest := EpayCheckoutRequest{TotalCents: 49999, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: "routing-small-payment"}
	smallQuote, err := QuoteEpayCheckout(DB, u1.Id, smallRequest, false)
	require.NoError(t, err)
	smallRequest.Token = smallQuote.Token
	smallOrder, err := CreateEpayCheckout(u1.Id, smallRequest)
	require.NoError(t, err)
	owners[smallOrder.Id] = smallOrder.UserId
	config.PrivateGatewayName = "Renamed provider"
	config.Gateways = []InvoiceEpayGateway{}
	require.NoError(t, SaveInvoiceEpayConfig(config))
	notes, err := LoadEpayRoutingExportNotes(DB, owners)
	require.NoError(t, err)
	assert.Len(t, notes, 3)
	assert.Equal(t, smallOrder.TradeNo+" | alipay | Gateway P | ", notes[smallOrder.Id])
	assert.NotContains(t, notes[smallOrder.Id], "对私网关")
	assert.NotContains(t, notes[smallOrder.Id], "Renamed provider")
	retried, err := CreateEpayCheckout(u1.Id, smallRequest)
	require.NoError(t, err)
	assert.Equal(t, smallOrder.EpayCredentials, retried.EpayCredentials)
	for _, order := range orders {
		assert.Contains(t, notes[order.Id], order.TradeNo)
		assert.Contains(t, notes[order.Id], "Issuer ")
		assert.NotContains(t, notes[order.Id], "key-")
		assert.NotContains(t, notes[order.Id], "https://")
		raw, err := common.Marshal(order)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "Issuer ")
	}
	notes, err = LoadEpayRoutingExportNotes(DB, map[int]int{orders[0].Id: -1})
	require.NoError(t, err)
	assert.Empty(t, notes)
}

func TestEpayRoutingConfigSecretsAndRandomEligibility(t *testing.T) {
	config := setupEpayRoutingTest(t)
	config.Gateways[0].Key, config.Gateways[1].Key = "", ""
	config.Strategy = "random"
	require.NoError(t, SaveInvoiceEpayConfig(config))
	stored, err := GetInvoiceEpayConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, "key-a", stored.Gateways[0].Key)
	config.Gateways[0].PartnerID = "different"
	assert.ErrorIs(t, SaveInvoiceEpayConfig(config), ErrInvoiceTaxInvalid)
	config.Gateways[0].PartnerID = "a"
	config.Gateways[1].ID = "corporate-a"
	assert.ErrorIs(t, SaveInvoiceEpayConfig(config), ErrInvoiceTaxInvalid)
	user := insertInvoiceTicketUser(t, 981105)
	quote, err := QuoteEpayCheckout(DB, user.Id, EpayCheckoutRequest{TotalCents: 50000, InvoiceType: "ordinary", PaymentMethod: "alipay"}, false)
	require.NoError(t, err)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		route, err := selectEpayCheckoutGateway(tx, quote.Routes, quote.Strategy, "alipay")
		assert.Contains(t, []string{"corporate-a", "corporate-b"}, route.GatewayID)
		return err
	}))
	assert.ErrorIs(t, SaveInvoiceEpayConfig(InvoiceEpayConfig{Address: "https://old.example.test", PartnerID: "old", Key: "old"}), ErrInvoiceTaxInvalid)
}

func TestInvoiceExportRoutingIsPerOrderAndAbsentFromFinanceList(t *testing.T) {
	setupEpayRoutingTest(t)
	user := insertInvoiceTicketUser(t, 981106)
	other := insertInvoiceTicketUser(t, 981107)
	var ids []int
	for _, input := range []struct {
		owner         int
		trade, issuer string
	}{
		{user.Id, "export-route-a", "Issuer A"}, {user.Id, "export-route-b", "Issuer B"}, {other.Id, "export-foreign", "Foreign issuer"},
	} {
		credentials, err := common.Marshal(InvoiceEpayConfig{GatewayID: input.trade, GatewayName: input.trade, AccountName: input.issuer, Key: "export-secret"})
		require.NoError(t, err)
		order := TopUp{UserId: input.owner, TradeNo: input.trade, Money: 500, PaymentMethod: "alipay", Status: common.TopUpStatusSuccess, EpayCredentials: string(credentials)}
		require.NoError(t, DB.Create(&order).Error)
		ids = append(ids, order.Id)
	}
	ticket := Ticket{UserId: user.Id, Type: TicketTypeInvoice, Status: TicketStatusOpen}
	require.NoError(t, DB.Create(&ticket).Error)
	invoice := TicketInvoice{TicketId: ticket.Id, UserId: user.Id, CompanyName: "Buyer", TotalMoney: 1000, InvoiceStatus: InvoiceStatusPending}
	require.NoError(t, invoice.SetTopUpOrderIDs(ids))
	require.NoError(t, DB.Create(&invoice).Error)
	items, _, err := ListInvoicesForExport(InvoiceExportFilter{}, &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Contains(t, items[0].PaymentRoutingNotes, "export-route-a | alipay | export-route-a | Issuer A")
	assert.Contains(t, items[0].PaymentRoutingNotes, "export-route-b | alipay | export-route-b | Issuer B")
	assert.NotContains(t, items[0].PaymentRoutingNotes, "Foreign")
	raw, err := common.Marshal(items)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "export-secret")
	list, _, err := ListFinanceRecords(context.Background(), FinanceFilter{View: "invoices"}, DefaultFinancePolicy(), &common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, list, 1)
	raw, err = common.Marshal(list)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "Issuer")
	assert.NotContains(t, string(raw), "payment_routing_notes")
}
