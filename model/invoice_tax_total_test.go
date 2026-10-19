package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceTaxFromTotalPreservesPaymentAmount(t *testing.T) {
	for _, test := range []struct {
		name, mode, invoiceType string
		total, base, fee        int64
	}{
		{"ordinary VAT", "vat", "ordinary", 1060000, 1000000, 60000},
		{"compound", "compound", "ordinary", 1123600, 1000000, 123600},
		{"gross up", "gross_up", "ordinary", 1063830, 1000000, 63830},
		{"special VAT", "vat", "special", 1130000, 1000000, 130000},
		{"fractional cent", "vat", "ordinary", 10000, 9434, 566},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := InvoiceTaxConfig{Enabled: true, Mode: test.mode, OrdinaryRateBps: 600, SpecialRateBps: 1300}
			quote, err := CalculateInvoiceTaxFromTotal(test.total, test.invoiceType, config)
			require.NoError(t, err)
			assert.Equal(t, test.base, quote.BaseCents)
			assert.Equal(t, test.fee, quote.FeeCents)
			assert.Equal(t, test.total, quote.TotalCents)
			assert.Equal(t, quote.TotalCents, quote.BaseCents+quote.FeeCents)
		})
	}
}

func TestInvoiceTaxFromTotalRejectsInvalidAmounts(t *testing.T) {
	config := InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600}
	for _, amount := range []int64{-1, 0, 100000000001} {
		_, err := CalculateInvoiceTaxFromTotal(amount, "ordinary", config)
		assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	}
	_, err := CalculateInvoiceTaxFromTotal(100, "unknown", config)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	config.Mode = "unknown"
	_, err = CalculateInvoiceTaxFromTotal(100, "ordinary", config)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
}
