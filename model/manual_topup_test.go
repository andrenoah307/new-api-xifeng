package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManualTopUpTest(t *testing.T) (*User, *User) {
	t.Helper()
	setupEpayRechargeTest(t)
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = old
		require.NoError(t, DB.Where("key = ?", InvoiceTaxConfigKey).Delete(&Option{}).Error)
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", OrdinaryRateBps: 600, SpecialRateBps: 1300}))
	admin := &User{Username: "manual-admin", AffCode: "manual-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	user := &User{Username: "manual-user", AffCode: "manual-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 123}
	require.NoError(t, DB.Create(admin).Error)
	require.NoError(t, DB.Create(user).Error)
	return admin, user
}

func TestManualTopUpAtomicIdempotentAndInvoiceable(t *testing.T) {
	admin, user := setupManualTopUpTest(t)
	req := ManualTopUpRequest{UserID: user.Id, TotalCents: 10000, InvoiceType: "ordinary", RequestID: "manual-request-0001"}
	quote, err := QuoteManualTopUp(DB, admin.Id, req)
	require.NoError(t, err)
	assert.Equal(t, int64(9434), quote.NetCents)
	assert.Equal(t, int64(566), quote.TaxCents)
	req.Token = quote.Token
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := CreateManualTopUp(admin.Id, req); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	order, created, err := CreateManualTopUp(admin.Id, req)
	require.NoError(t, err)
	assert.False(t, created)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 123+47170000, user.Quota)
	assert.Equal(t, int64(47170000), order.QuotaGranted)
	assert.Equal(t, "manual", order.Source)
	assert.Equal(t, admin.Id, order.CreatedBy)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	eligible, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	tax, err := QuoteInvoiceTax(DB, user.Id, []int{order.Id}, "ordinary")
	require.NoError(t, err)
	assert.True(t, tax.PrepaidOnly)
	assert.Zero(t, tax.FeeCents)
	params := createInvoiceTicketParams(user, []int{order.Id})
	params.InvoiceType = "ordinary"
	_, invoice, _, _, err := CreateInvoiceTicket(params)
	require.NoError(t, err)
	assert.Equal(t, float64(100), invoice.TotalMoney)
	assert.Equal(t, int64(566), invoice.InvoiceTaxCents)
	assert.Nil(t, invoice.TaxCharge)
	req.TotalCents++
	_, _, err = CreateManualTopUp(admin.Id, req)
	assert.Error(t, err)
}

func TestManualTopUpConcurrentReceiptsPreserveOtherWalletWrites(t *testing.T) {
	admin, user := setupManualTopUpTest(t)
	req := ManualTopUpRequest{UserID: user.Id, TotalCents: 100, InvoiceType: "none", RequestID: "manual-concurrent-0001"}
	quote, err := QuoteManualTopUp(DB, admin.Id, req)
	require.NoError(t, err)
	req.Token = quote.Token
	second := req
	second.RequestID = "manual-concurrent-0002"
	start := make(chan struct{})
	results := make(chan error, 3)
	go func() { <-start; _, _, err := CreateManualTopUp(admin.Id, req); results <- err }()
	go func() { <-start; _, _, err := CreateManualTopUp(admin.Id, second); results <- err }()
	go func() {
		<-start
		results <- DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", gorm.Expr("quota - ?", 100)).Error
	}()
	close(start)
	for i := 0; i < 3; i++ {
		require.NoError(t, <-results)
	}
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 1000023, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestManualTopUpRollbackPermissionsAndQuoteChanges(t *testing.T) {
	admin, user := setupManualTopUpTest(t)
	req := ManualTopUpRequest{UserID: user.Id, TotalCents: 10000, InvoiceType: "special", RequestID: "manual-request-0002"}
	_, err := QuoteManualTopUp(DB, user.Id, req)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	req.UserID = admin.Id
	_, err = QuoteManualTopUp(DB, admin.Id, req)
	assert.ErrorIs(t, err, ErrTicketForbidden)
	req.UserID = user.Id
	quote, err := QuoteManualTopUp(DB, admin.Id, req)
	require.NoError(t, err)
	req.Token = quote.Token
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{Enabled: true, Mode: "vat", SpecialRateBps: 600}))
	_, _, err = CreateManualTopUp(admin.Id, req)
	assert.ErrorIs(t, err, ErrInvoiceTaxQuoteChanged)
	req.InvoiceType = "none"
	quote, err = QuoteManualTopUp(DB, admin.Id, req)
	require.NoError(t, err)
	req.Token = quote.Token
	assert.Zero(t, quote.TaxCents)
	assert.Equal(t, int64(10000), quote.NetCents)
	injected := errors.New("injected wallet failure")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:manual_wallet_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(injected)
		}
	}))
	t.Cleanup(func() { DB.Callback().Update().Remove("test:manual_wallet_failure") })
	_, _, err = CreateManualTopUp(admin.Id, req)
	assert.ErrorIs(t, err, injected)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 123, user.Quota)
	require.NoError(t, DB.Callback().Update().Remove("test:manual_wallet_failure"))
	require.NoError(t, DB.Model(user).Update("quota", common.MaxQuota).Error)
	_, _, err = CreateManualTopUp(admin.Id, req)
	assert.Error(t, err)
	require.NoError(t, DB.Model(&TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(user).Update("quota", 0).Error)
	order, created, err := CreateManualTopUp(admin.Id, req)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "none", order.InvoiceType)
	eligible, err := GetEligibleInvoiceOrders(user.Id, "ordinary")
	require.NoError(t, err)
	assert.Empty(t, eligible)
}
