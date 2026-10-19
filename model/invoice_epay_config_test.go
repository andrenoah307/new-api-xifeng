package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceEpayConfigPreservesSecretAndRejectsPartialMerchantChanges(t *testing.T) {
	require.NoError(t, DB.Where(Option{Key: InvoiceEpayConfigKey}).Delete(&Option{}).Error)
	t.Cleanup(func() { DB.Where(Option{Key: InvoiceEpayConfigKey}).Delete(&Option{}) })
	config := InvoiceEpayConfig{Address: "https://pay.example.test", PartnerID: "merchant-a", Key: "test-secret"}
	require.NoError(t, SaveInvoiceEpayConfig(config))
	unchanged := config
	unchanged.Key = ""
	require.NoError(t, SaveInvoiceEpayConfig(unchanged))
	actual, err := GetInvoiceEpayConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, config, actual)
	unchanged.PartnerID = "merchant-b"
	require.ErrorIs(t, SaveInvoiceEpayConfig(unchanged), ErrInvoiceTaxInvalid)
	actual, err = GetInvoiceEpayConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, config, actual)
	unchanged.Key = "new-secret"
	require.NoError(t, SaveInvoiceEpayConfig(unchanged))
	actual, err = GetInvoiceEpayConfig(DB)
	require.NoError(t, err)
	assert.Equal(t, unchanged, actual)
}

func TestInvoiceEpayConfigRejectsIncompleteInitialConfiguration(t *testing.T) {
	require.NoError(t, DB.Where(Option{Key: InvoiceEpayConfigKey}).Delete(&Option{}).Error)
	config := InvoiceEpayConfig{Address: "https://pay.example.test", PartnerID: "merchant"}
	require.ErrorIs(t, SaveInvoiceEpayConfig(config), ErrInvoiceTaxInvalid)
	var count int64
	require.NoError(t, DB.Model(&Option{}).Where(Option{Key: InvoiceEpayConfigKey}).Count(&count).Error)
	assert.Zero(t, count)
}
