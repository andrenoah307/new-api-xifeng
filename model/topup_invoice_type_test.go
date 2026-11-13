package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func invoiceAdjustmentFixture(t *testing.T) (*User, *User, *TopUp) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range []any{&TicketInvoice{}, &InvoiceTaxCharge{}, &TicketRefund{}, &TicketMessage{}, &Ticket{}, &TopUp{}, &User{}, &Log{}} {
			require.NoError(t, DB.Unscoped().Where("1=1").Delete(table).Error)
		}
	})
	require.NoError(t, DB.AutoMigrate(&TopUpInvoiceAdjustment{}))
	require.NoError(t, DB.Where("1=1").Delete(&TopUpInvoiceAdjustment{}).Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Where("1=1").Delete(&TopUpInvoiceAdjustment{}).Error)
		require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error)
	})
	oldUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldUnit })
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt, RefundOverdraftLimitCents: 2000}))
	owner := insertInvoiceTicketUser(t, 980601)
	actor := insertInvoiceTicketUser(t, 980602)
	require.NoError(t, DB.Model(actor).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, DB.Model(owner).Update("quota", 1000000).Error)
	order := &TopUp{UserId: owner.Id, TradeNo: "invoice-adjustment", Status: common.TopUpStatusSuccess, Money: 100, PaymentProvider: PaymentProviderEpay, InvoiceType: "none", CheckoutVersion: 1, PaymentTotalCents: 10000, InvoiceNetCents: 10000, QuotaGranted: 50000000, CheckoutBaseQuota: 50000000, CheckoutQuota: 51000000, CompleteTime: InvoicePolicyEffectiveAt + 1}
	require.NoError(t, DB.Create(order).Error)
	return owner, actor, order
}

func TestInvoiceAdjustmentDebitsConfirmsAndReverses(t *testing.T) {
	owner, actor, order := invoiceAdjustmentFixture(t)
	expected := "none"
	input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "special"}
	preview, err := ManageTopUpInvoiceType(actor.Id, input, false)
	require.NoError(t, err)
	assert.Equal(t, int64(900), preview.TaxAfterCents)
	assert.Equal(t, "-4500000", preview.QuotaDelta)
	assert.Equal(t, "-7", preview.BalanceAfter)
	assert.True(t, preview.RequiresNegativeConfirmation)
	input.Revision = preview.Revision
	_, err = ManageTopUpInvoiceType(actor.Id, input, true)
	assert.ErrorIs(t, err, ErrInvoiceAdjustmentNegative)
	require.NoError(t, DB.First(owner, owner.Id).Error)
	assert.Equal(t, 1000000, owner.Quota)
	input.ConfirmNegative = true
	done, err := ManageTopUpInvoiceType(actor.Id, input, true)
	require.NoError(t, err)
	assert.True(t, done.Applied)
	again, err := ManageTopUpInvoiceType(actor.Id, input, true)
	require.NoError(t, err)
	assert.Equal(t, done, again)
	require.NoError(t, DB.First(owner, owner.Id).Error)
	assert.Equal(t, -3500000, owner.Quota)
	require.NoError(t, DB.First(order, order.Id).Error)
	assert.Equal(t, int64(45500000), order.QuotaGranted)
	assert.Equal(t, 46500000, order.CheckoutQuota)
	assert.Equal(t, float64(100), order.Money)
	assert.Equal(t, int64(10000), order.PaymentTotalCents)
	quote, err := QuoteInvoiceTax(DB, owner.Id, []int{order.Id}, "special")
	require.NoError(t, err)
	assert.True(t, quote.PrepaidOnly)
	assert.Zero(t, quote.FeeCents)
	assert.Equal(t, int64(900), quote.PrepaidTaxCents)
	expected = "special"
	input.InvoiceType = "none"
	preview, err = ManageTopUpInvoiceType(actor.Id, input, false)
	require.NoError(t, err)
	assert.Equal(t, "9", preview.BalanceChange)
	input.Revision = preview.Revision
	_, err = ManageTopUpInvoiceType(actor.Id, input, true)
	require.NoError(t, err)
	require.NoError(t, DB.First(owner, owner.Id).Error)
	assert.Equal(t, 1000000, owner.Quota)
	var receipts []TopUpInvoiceAdjustment
	require.NoError(t, DB.Find(&receipts).Error)
	require.Len(t, receipts, 2)
	assert.Contains(t, receipts[0].BeforeJSON, `"invoice_type":"none"`)
	assert.NotContains(t, receipts[0].BeforeJSON, "epay_credentials")
}

func TestInvoiceAdjustmentGuardsAndRollback(t *testing.T) {
	for _, scenario := range []string{"stale wallet", "stale setting", "disabled admin", "finance role", "grace exceeded", "no allowance", "invoice occupied", "refund pending", "refund completed", "legacy snapshot", "receipt failure"} {
		t.Run(scenario, func(t *testing.T) {
			owner, actor, order := invoiceAdjustmentFixture(t)
			expected := "none"
			input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "special", ConfirmNegative: true}
			preview, err := ManageTopUpInvoiceType(actor.Id, input, false)
			require.NoError(t, err)
			input.Revision = preview.Revision
			switch scenario {
			case "stale wallet":
				require.NoError(t, DB.Model(owner).Update("quota", 999999).Error)
			case "stale setting":
				require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt, RefundOverdraftLimitCents: 2100}))
			case "disabled admin":
				require.NoError(t, DB.Model(actor).Update("status", common.UserStatusDisabled).Error)
			case "finance role":
				require.NoError(t, DB.Model(actor).Update("role", common.RoleFinanceUser).Error)
			case "grace exceeded", "no allowance":
				limit := int64(699)
				if scenario == "no allowance" {
					limit = 0
				}
				require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt, RefundOverdraftLimitCents: limit}))
				preview, err = ManageTopUpInvoiceType(actor.Id, input, false)
				require.NoError(t, err)
				assert.False(t, preview.CanApply)
				input.Revision = preview.Revision
			case "invoice occupied":
				invoice := TicketInvoice{UserId: owner.Id, TicketId: 777, InvoiceStatus: InvoiceStatusPending}
				require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
				require.NoError(t, DB.Create(&invoice).Error)
			case "refund pending", "refund completed":
				status := RefundStatusPending
				if scenario == "refund completed" {
					status = RefundStatusRefunded
				}
				require.NoError(t, DB.Create(&TicketRefund{TicketId: 778, UserId: owner.Id, RefundStatus: status}).Error)
			case "legacy snapshot":
				require.NoError(t, DB.Model(order).Update("checkout_version", 0).Error)
			case "receipt failure":
				require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("reject_invoice_adjustment_receipt", func(tx *gorm.DB) {
					if tx.Statement.Table == "top_up_invoice_adjustments" {
						tx.AddError(errors.New("receipt unavailable"))
					}
				}))
				t.Cleanup(func() { require.NoError(t, DB.Callback().Create().Remove("reject_invoice_adjustment_receipt")) })
			}
			_, err = ManageTopUpInvoiceType(actor.Id, input, true)
			require.Error(t, err)
			require.NoError(t, DB.First(order, order.Id).Error)
			assert.Equal(t, "none", order.InvoiceType)
			assert.Zero(t, order.InvoiceTaxCents)
			require.NoError(t, DB.First(owner, owner.Id).Error)
			want := 1000000
			if scenario == "stale wallet" {
				want = 999999
			}
			assert.Equal(t, want, owner.Quota)
			var count int64
			require.NoError(t, DB.Model(&TopUpInvoiceAdjustment{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestInvoiceAdjustmentAndApplicationCannotBothSucceed(t *testing.T) {
	owner, actor, order := invoiceAdjustmentFixture(t)
	require.NoError(t, DB.Model(order).Update("invoice_type", "ordinary").Error)
	oldMinimum := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 0
	t.Cleanup(func() { operation_setting.MinInvoiceAmount = oldMinimum })
	expected := "ordinary"
	input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "none"}
	preview, err := ManageTopUpInvoiceType(actor.Id, input, false)
	require.NoError(t, err)
	input.Revision = preview.Revision
	params := createInvoiceTicketParams(owner, []int{order.Id})
	params.InvoiceType = "ordinary"
	editResult, invoiceResult := make(chan error, 1), make(chan error, 1)
	start := make(chan struct{})
	go func() { <-start; _, err := ManageTopUpInvoiceType(actor.Id, input, true); editResult <- err }()
	go func() { <-start; _, _, _, _, err := CreateInvoiceTicket(params); invoiceResult <- err }()
	close(start)
	editErr, invoiceErr := <-editResult, <-invoiceResult
	if editErr == nil {
		assert.ErrorIs(t, invoiceErr, ErrTicketInvoiceOrderInvalid)
	} else {
		assert.ErrorIs(t, editErr, ErrTicketInvoiceOrderDuplicate)
		require.NoError(t, invoiceErr)
	}
}

func TestInvoiceAdjustmentRoundingAndConfiguredAllowance(t *testing.T) {
	t.Run("historical ordinary exemption", func(t *testing.T) {
		owner, actor, order := invoiceAdjustmentFixture(t)
		require.NoError(t, DB.Model(order).Update("complete_time", InvoicePolicyEffectiveAt-1).Error)
		expected := "none"
		input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "ordinary"}
		plan, err := ManageTopUpInvoiceType(actor.Id, input, false)
		require.NoError(t, err)
		assert.Zero(t, plan.TaxAfterCents)
		assert.Equal(t, "0", plan.QuotaDelta)
		input.Revision = plan.Revision
		_, err = ManageTopUpInvoiceType(actor.Id, input, true)
		require.NoError(t, err)
		require.NoError(t, DB.First(owner, owner.Id).Error)
		assert.Equal(t, 1000000, owner.Quota)
	})
	t.Run("allowance above ten", func(t *testing.T) {
		owner, actor, order := invoiceAdjustmentFixture(t)
		require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"money": 200, "payment_total_cents": 20000, "invoice_net_cents": 20000, "quota_granted": 100000000, "checkout_base_quota": 100000000, "checkout_quota": 100000000}).Error)
		expected := "none"
		input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "special", ConfirmNegative: true}
		plan, err := ManageTopUpInvoiceType(actor.Id, input, false)
		require.NoError(t, err)
		assert.Equal(t, "-16", plan.BalanceAfter)
		assert.True(t, plan.CanApply)
		input.Revision = plan.Revision
		_, err = ManageTopUpInvoiceType(actor.Id, input, true)
		require.NoError(t, err)
		require.NoError(t, DB.First(owner, owner.Id).Error)
		assert.Equal(t, -8000000, owner.Quota)
	})
	t.Run("round trip and duplicate submission", func(t *testing.T) {
		owner, actor, order := invoiceAdjustmentFixture(t)
		require.NoError(t, DB.Model(order).Updates(map[string]interface{}{"quota_granted": 17, "checkout_base_quota": 17, "checkout_quota": 17}).Error)
		expected := "none"
		input := TopUpInvoiceAdjustmentInput{ID: order.Id, ExpectedInvoiceType: &expected, InvoiceType: "special"}
		plan, err := ManageTopUpInvoiceType(actor.Id, input, false)
		require.NoError(t, err)
		input.Revision = plan.Revision
		results := make(chan error, 2)
		start := make(chan struct{})
		for range 2 {
			go func() { <-start; _, err := ManageTopUpInvoiceType(actor.Id, input, true); results <- err }()
		}
		close(start)
		require.NoError(t, <-results)
		require.NoError(t, <-results)
		require.NoError(t, DB.First(owner, owner.Id).Error)
		assert.Equal(t, 999998, owner.Quota)
		expected = "special"
		input.InvoiceType = "none"
		plan, err = ManageTopUpInvoiceType(actor.Id, input, false)
		require.NoError(t, err)
		input.Revision = plan.Revision
		_, err = ManageTopUpInvoiceType(actor.Id, input, true)
		require.NoError(t, err)
		require.NoError(t, DB.First(owner, owner.Id).Error)
		assert.Equal(t, 1000000, owner.Quota)
		var count int64
		require.NoError(t, DB.Model(&TopUpInvoiceAdjustment{}).Count(&count).Error)
		assert.Equal(t, int64(2), count)
	})
}
