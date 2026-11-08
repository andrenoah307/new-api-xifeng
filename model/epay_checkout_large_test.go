package model

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEpayCheckoutLargeQuotaPrecision(t *testing.T) {
	config := InvoiceTaxConfig{Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600, SpecialRateBps: 900}
	for _, tc := range []struct {
		name, kind, price, unit string
		cents                   int64
		rate                    int
		net, tax                int64
		base, quota             int
	}{
		{"5000 ordinary", "ordinary", "1", "500000", 500000, 100, 470000, 30000, 2350000000, 2350000000},
		{"5000 special", "special", "1", "500000", 500000, 100, 455000, 45000, 2275000000, 2275000000},
		{"100000 ordinary", "ordinary", "1", "500000", 10000000, 100, 9400000, 600000, 47000000000, 47000000000},
		{"100000 special", "special", "1", "500000", 10000000, 100, 9100000, 900000, 45500000000, 45500000000},
		{"100000 no invoice", "none", "1", "500000", 10000000, 100, 10000000, 0, 50000000000, 50000000000},
		{"cents and discount", "special", "1", "500000", 10000001, 90, 9100001, 900000, 45500005000, 50555561111},
		{"half quota rounds up", "none", "2", "100", 101, 100, 101, 0, 51, 51},
		{"above half without intermediate rounding", "none", "1.99999999999999999999", "1", 100, 100, 100, 0, 1, 1},
		{"JSON exact limit", "none", "1", "9007199254740991", 100, 100, 100, 0, 9007199254740991, 9007199254740991},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := calculateEpayCheckout(tc.cents, tc.kind, config, decimal.RequireFromString(tc.price), tc.rate, decimal.RequireFromString(tc.unit))
			require.NoError(t, err)
			assert.Equal(t, tc.net, quote.NetCents)
			assert.Equal(t, tc.tax, quote.TaxCents)
			assert.Equal(t, tc.cents, quote.NetCents+quote.TaxCents)
			assert.Equal(t, tc.base, quote.BaseQuota)
			assert.Equal(t, tc.quota, quote.Quota)
			raw, err := common.Marshal(quote)
			require.NoError(t, err)
			var decoded EpayCheckoutQuote
			require.NoError(t, common.Unmarshal(raw, &decoded))
			assert.Equal(t, quote.Quota, decoded.Quota)
			assert.Equal(t, quote.CreditAmount, decoded.CreditAmount)
		})
	}
	for _, tc := range []struct {
		name, price, unit string
		rate              int
	}{
		{"JSON precision overflow", "1", "9007199254740992", 100},
		{"discount precision overflow", "1", "9007199254740991", 99},
		{"int64 overflow", "1", "9223372036854775808", 100},
		{"whole credit overflow", "0.00000000000000000001", "0.00000000000000000001", 100},
		{"below half without intermediate rounding", "2.00000000000000000001", "1", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := calculateEpayCheckout(100, "none", config, decimal.RequireFromString(tc.price), tc.rate, decimal.RequireFromString(tc.unit))
			assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
		})
	}
}

func setupLargeCheckoutTest(t *testing.T) {
	t.Helper()
	models := []any{&Option{}, &User{}, &TopUp{}, &DiscountCode{}, &DiscountCodeUsage{}, &Log{}}
	require.NoError(t, DB.AutoMigrate(models...))
	oldPayment, oldCode := *operation_setting.GetPaymentSetting(), *operation_setting.GetDiscountCodeSetting()
	oldPrice, oldMin, oldUnit := operation_setting.Price, operation_setting.MinTopUp, common.QuotaPerUnit
	oldAddress, oldID, oldKey, oldMethods := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	oldRedis := common.RedisEnabled
	common.RedisEnabled, common.QuotaPerUnit = false, 500000
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
	operation_setting.GetDiscountCodeSetting().Enabled = true
	operation_setting.Price, operation_setting.MinTopUp = 1, 1
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "https://private.example.test", "private", "private-key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() {
		*operation_setting.GetPaymentSetting(), *operation_setting.GetDiscountCodeSetting() = oldPayment, oldCode
		operation_setting.Price, operation_setting.MinTopUp, common.QuotaPerUnit = oldPrice, oldMin, oldUnit
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods = oldAddress, oldID, oldKey, oldMethods
		common.RedisEnabled = oldRedis
		for _, model := range models {
			require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(model).Error)
		}
	})
	require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, Enabled: true, Mode: "gross_up", OrdinaryRateBps: 600, SpecialRateBps: 900}))
	require.NoError(t, SaveInvoiceEpayConfig(InvoiceEpayConfig{Strategy: "round_robin", PrivateBelowCents: 500000, PrivateGatewayName: "Private", Gateways: []InvoiceEpayGateway{
		{ID: "corporate-a", Name: "Corporate", AccountName: "Issuer", Address: "https://corporate.example.test", PartnerID: "corporate", Key: "corporate-key", PaymentMethods: []string{"alipay"}},
	}}))
}

func TestEpayCheckoutLargePayments(t *testing.T) {
	for _, tc := range []struct {
		name, kind      string
		cents           int64
		base, quota     int
		discount        bool
		balanceOverflow bool
	}{
		{"5000 corporate", "ordinary", 500000, 2350000000, 2350000000, false, false},
		{"100000 corporate", "ordinary", 10000000, 47000000000, 47000000000, false, false},
		{"100000 cents and bonus", "special", 10000001, 45500005000, 50555561111, true, false},
		{"100000 private", "none", 10000001, 50000005000, 50000005000, false, false},
		{"wallet overflow rolls back bonus", "special", 10000001, 45500005000, 50555561111, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupLargeCheckoutTest(t)
			user := insertInvoiceTicketUser(t, 981201)
			// Odd balances near bigint's limit detect even a single lost quota unit.
			balance := math.MaxInt64 - tc.quota
			if tc.balanceOverflow {
				balance++
			}
			require.NoError(t, DB.Model(user).Update("quota", balance).Error)
			req := EpayCheckoutRequest{TotalCents: tc.cents, InvoiceType: tc.kind, PaymentMethod: "alipay", RequestID: "large-checkout-request"}
			if tc.discount {
				code := DiscountCode{Code: "large-checkout-90", DiscountRate: 90, Status: DiscountCodeStatusEnabled}
				require.NoError(t, DB.Create(&code).Error)
				t.Cleanup(func() {
					require.NoError(t, DB.Where("discount_code_id = ?", code.Id).Delete(&DiscountCodeUsage{}).Error)
					require.NoError(t, DB.Unscoped().Delete(&code).Error)
				})
				req.DiscountCode = code.Code
			}
			quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
			require.NoError(t, err)
			assert.Equal(t, tc.base, quote.BaseQuota)
			assert.Equal(t, tc.quota, quote.Quota)
			req.Token = quote.Token
			order, err := CreateEpayCheckout(user.Id, req)
			require.NoError(t, err)
			assert.Regexp(t, fmt.Sprintf(`^USR%dNO[0-9a-zA-Z]{6}%d$`, user.Id, order.CreateTime), order.TradeNo)
			var credentials InvoiceEpayConfig
			require.NoError(t, common.UnmarshalJsonStr(order.EpayCredentials, &credentials))
			if tc.kind == "none" {
				assert.Equal(t, "private", credentials.GatewayID)
			} else {
				assert.Equal(t, "corporate-a", credentials.GatewayID)
			}
			duplicate, err := CreateEpayCheckout(user.Id, req)
			require.NoError(t, err)
			assert.Equal(t, order.Id, duplicate.Id)
			_, credited, err := CompleteEpayCheckout(order.TradeNo, "alipay", tc.cents-1)
			require.ErrorIs(t, err, ErrInvoiceTaxInvalid)
			assert.Zero(t, credited)
			_, credited, err = CompleteEpayCheckout(order.TradeNo, "alipay", tc.cents)
			if tc.balanceOverflow {
				require.ErrorIs(t, err, ErrInvoiceTaxInvalid)
				assert.Zero(t, credited)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.quota, credited)
				_, credited, err = CompleteEpayCheckout(order.TradeNo, "alipay", tc.cents)
				require.NoError(t, err)
				assert.Zero(t, credited)
				require.NoError(t, ManualCompleteTopUp(order.TradeNo, "127.0.0.1"))
			}
			require.NoError(t, DB.First(user, user.Id).Error)
			require.NoError(t, DB.First(order, order.Id).Error)
			var gifts []TopUp
			require.NoError(t, DB.Where("trade_no = ?", order.TradeNo+"_bonus").Find(&gifts).Error)
			if tc.balanceOverflow {
				assert.Equal(t, balance, user.Quota)
				assert.Equal(t, common.TopUpStatusPending, order.Status)
				assert.Zero(t, order.QuotaGranted)
				assert.Empty(t, gifts)
			} else {
				assert.Equal(t, int(math.MaxInt64), user.Quota)
				assert.Equal(t, common.TopUpStatusSuccess, order.Status)
				assert.Equal(t, int64(tc.base), order.QuotaGranted)
				if tc.discount {
					require.Len(t, gifts, 1)
					assert.Equal(t, int64(tc.quota-tc.base), gifts[0].QuotaGranted)
				} else {
					assert.Empty(t, gifts)
				}
			}
		})
	}
}

func TestEpayCheckoutOrderNumberCompatibility(t *testing.T) {
	setupLargeCheckoutTest(t)
	user := insertInvoiceTicketUser(t, 981202)
	req := EpayCheckoutRequest{TotalCents: 500000, InvoiceType: "ordinary", PaymentMethod: "alipay", RequestID: strings.Repeat("a", 48)}
	quote, err := QuoteEpayCheckout(DB, user.Id, req, false)
	require.NoError(t, err)
	req.Token = quote.Token
	var orders [2]*TopUp
	var errs [2]error
	var calls sync.WaitGroup
	for i := range orders {
		calls.Add(1)
		go func(i int) {
			defer calls.Done()
			orders[i], errs[i] = CreateEpayCheckout(user.Id, req)
		}(i)
	}
	calls.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, orders[0].Id, orders[1].Id)
	assert.Len(t, orders[0].TradeNo, len(fmt.Sprintf("USR%dNO", user.Id))+6+len(fmt.Sprint(orders[0].CreateTime)))
	assert.Regexp(t, fmt.Sprintf(`^USR%dNO[0-9a-zA-Z]{6}%d$`, user.Id, orders[0].CreateTime), orders[0].TradeNo)
	changed := req
	changed.TotalCents++
	_, err = CreateEpayCheckout(user.Id, changed)
	assert.ErrorIs(t, err, ErrInvoiceTaxInvalid)
	changed = req
	changed.RequestID = strings.Repeat("b", 48)
	second, err := CreateEpayCheckout(user.Id, changed)
	require.NoError(t, err)
	assert.NotEqual(t, orders[0].TradeNo, second.TradeNo)

	// Upgrade compatibility: the prior release encoded request identity in
	// the order number and left the new nullable unique key absent.
	legacyNo := fmt.Sprintf("CHECKOUT%d-%s", user.Id, req.RequestID)
	require.NoError(t, DB.Model(orders[0]).Updates(map[string]any{"trade_no": legacyNo, "checkout_request_id": nil}).Error)
	legacy, err := CreateEpayCheckout(user.Id, req)
	require.NoError(t, err)
	assert.Equal(t, orders[0].Id, legacy.Id)
	assert.Equal(t, legacyNo, legacy.TradeNo)
	_, credited, err := CompleteEpayCheckout(legacyNo, "alipay", req.TotalCents)
	require.NoError(t, err)
	assert.Equal(t, 2350000000, credited)
	require.NoError(t, ManualCompleteTopUp(legacyNo, "127.0.0.1"))
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 2350000000, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestEpayCheckoutRequestIDMigration(t *testing.T) {
	const table = "checkout_upgrade_top_ups"
	// Model the pre-upgrade columns in a separate disposable table.
	type legacyOrder struct {
		Id           int
		UserId       int
		TradeNo      string `gorm:"type:varchar(255)"`
		QuotaGranted int64
	}
	require.NoError(t, DB.Table(table).AutoMigrate(&legacyOrder{}))
	t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(table)) })
	old := []legacyOrder{
		{Id: 1, UserId: 12345, TradeNo: "CHECKOUT12345-old-request", QuotaGranted: 47000005000},
		{Id: 2, UserId: 12345, TradeNo: "USR12345NOAb12Cd1794153000", QuotaGranted: 50000005000},
	}
	require.NoError(t, DB.Table(table).Create(&old).Error)
	require.NoError(t, DB.Table(table).AutoMigrate(&TopUp{}))
	require.NoError(t, DB.Table(table).AutoMigrate(&TopUp{}))
	var upgraded []TopUp
	require.NoError(t, DB.Table(table).Order("id").Find(&upgraded).Error)
	require.Len(t, upgraded, 2)
	for i, order := range upgraded {
		assert.Equal(t, old[i].TradeNo, order.TradeNo)
		assert.Equal(t, old[i].QuotaGranted, order.QuotaGranted)
		assert.Nil(t, order.CheckoutRequestID)
	}
	require.NoError(t, DB.Table(table).Where("id = ?", 1).Update("checkout_request_id", "same-request").Error)
	require.Error(t, DB.Table(table).Where("id = ?", 2).Update("checkout_request_id", "same-request").Error)
}

func TestEpayCheckoutLargePolicyRepricing(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		name := "legacy ratio"
		if snapshot {
			name = "frozen price"
		}
		t.Run(name, func(t *testing.T) {
			setupLargeCheckoutTest(t)
			require.NoError(t, SaveInvoiceTaxConfig(InvoiceTaxConfig{ModuleEnabled: true, PolicyEffectiveAt: InvoicePolicyEffectiveAt}))
			t.Cleanup(func() { require.NoError(t, DB.Where(Option{Key: InvoiceTaxConfigKey}).Delete(&Option{}).Error) })
			user, order := insertEpayRechargeFixture(t, "large-policy", PaymentProviderEpay, common.TopUpStatusExpired, 100000, 100000.01, 0)
			order.CheckoutVersion, order.InvoiceType, order.DiscountRate = 1, "ordinary", 100
			order.PaymentTotalCents, order.InvoiceNetCents = 10000001, 10000001
			order.CheckoutBaseQuota, order.CheckoutQuota = 50000005000, 50000005000
			if snapshot {
				order.CheckoutPolicyEffectiveAt = InvoicePolicyEffectiveAt
				order.CheckoutUnitPrice, order.CheckoutQuotaPerUnit = "1", "500000"
				// Completion must use the frozen unit rather than a changed setting.
				common.QuotaPerUnit = 1
			}
			require.NoError(t, DB.Save(order).Error)
			paid, credited, err := completeEpayCheckoutAt(order.TradeNo, "alipay", 10000001, InvoicePolicyEffectiveAt)
			require.NoError(t, err)
			assert.Equal(t, 47000005000, credited)
			assert.Equal(t, int64(9400001), paid.InvoiceNetCents)
			assert.Equal(t, int64(600000), paid.InvoiceTaxCents)
			assert.Equal(t, "94000.01", paid.CheckoutCreditAmount)
			_, credited, err = completeEpayCheckoutAt(order.TradeNo, "alipay", 10000001, InvoicePolicyEffectiveAt+1)
			require.NoError(t, err)
			assert.Zero(t, credited)
			require.NoError(t, DB.First(user, user.Id).Error)
			assert.Equal(t, 47000005100, user.Quota)
		})
	}
}

// DSNs must name disposable local databases, never an application database.
func TestEpayCheckoutLargePaymentsSQL(t *testing.T) {
	for _, engine := range []struct {
		name, env string
		kind      common.DatabaseType
	}{
		{"mysql", "TEST_CHECKOUT_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "TEST_CHECKOUT_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip("isolated SQL integration DSN not configured")
			}
			var dialect gorm.Dialector = mysql.Open(dsn)
			if engine.kind == common.DatabaseTypePostgreSQL {
				dialect = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			previousDB, previousLogDB := DB, LOG_DB
			mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(engine.kind, engine.kind)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(mainType, logType)
				initCol()
				require.NoError(t, conn.Close())
			})
			require.NoError(t, db.AutoMigrate(&Log{}))
			TestEpayCheckoutLargePayments(t)
			TestEpayCheckoutLargePolicyRepricing(t)
			TestEpayCheckoutOrderNumberCompatibility(t)
			TestEpayCheckoutRequestIDMigration(t)
		})
	}
}
