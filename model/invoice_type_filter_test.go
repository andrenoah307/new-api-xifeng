package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceTypeFilterPreservesTypedPaymentSnapshots(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
	config := InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600, SpecialRateBps: 900, LegacyFreeOrdinaryEnabled: true, LegacyInvoiceBefore: InvoicePolicyEffectiveAt}
	require.NoError(t, SaveInvoiceTaxConfig(config))
	user := insertInvoiceTicketUser(t, 980706)
	ids := make(map[string]int)
	for _, kind := range []string{"ordinary", "special", "none", ""} {
		id := seedInvoiceTicketTopUp(t, user.Id, "type-filter-"+kind, 100)
		updates := map[string]any{"invoice_type": kind, "complete_time": InvoicePolicyEffectiveAt - 1, "payment_provider": PaymentProviderEpay}
		if kind == "ordinary" || kind == "special" {
			updates["checkout_version"] = 1
			updates["invoice_net_cents"] = 9100
			updates["invoice_tax_cents"] = 900
			updates["payment_total_cents"] = 10000
		}
		require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", id).Updates(updates).Error)
		ids[kind] = id
	}
	for _, free := range []bool{true, false} {
		config.LegacyFreeOrdinaryEnabled = free
		require.NoError(t, SaveInvoiceTaxConfig(config))
		for _, kind := range []string{"ordinary", "special"} {
			eligible, err := GetEligibleInvoiceOrders(user.Id, kind)
			require.NoError(t, err)
			got := make([]int, 0, len(eligible))
			for _, order := range eligible {
				got = append(got, order.Id)
			}
			want := []int{ids[kind]}
			if free && kind == "ordinary" {
				want = append(want, ids[""], ids["none"])
			} else if !free {
				want = append(want, ids[""])
			}
			assert.ElementsMatch(t, want, got)
			quote, err := QuoteInvoiceTax(DB, user.Id, []int{ids[kind]}, kind)
			require.NoError(t, err)
			assert.Equal(t, int64(9100), quote.PrepaidNetCents)
			assert.Equal(t, int64(900), quote.PrepaidTaxCents)
			assert.Zero(t, quote.FeeCents)
			_, err = QuoteInvoiceTax(DB, user.Id, []int{ids["ordinary"], ids["special"]}, kind)
			assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
		}
	}
	config.LegacyFreeOrdinaryEnabled = true
	require.NoError(t, SaveInvoiceTaxConfig(config))
	params := createInvoiceTicketParams(user, []int{ids["special"]})
	params.InvoiceType = "ordinary"
	_, _, _, _, err := CreateInvoiceTicket(params)
	assert.ErrorIs(t, err, ErrTicketInvoiceOrderInvalid)
	params.InvoiceType = "special"
	_, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, "special", invoice.InvoiceType)
	assert.Equal(t, int64(900), invoice.InvoiceTaxCents)
	assert.Equal(t, float64(100), invoice.TotalMoney)
	assert.Nil(t, invoice.TaxCharge)
}
