package controller

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundInvoiceExportRolesAndReissueIdentity(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TicketRefund{}, &model.RefundInvoiceExport{}, &model.TopUp{}, &model.Option{}, &model.Ticket{}))
	previousService := operation_setting.InvoiceServiceName
	operation_setting.InvoiceServiceName = "=API service"
	t.Cleanup(func() { operation_setting.InvoiceServiceName = previousService })
	viewer := model.User{Username: "export-viewer", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "export-viewer"}
	require.NoError(t, db.Create(&viewer).Error)
	require.NoError(t, db.Create(&model.Ticket{Id: 99, UserId: viewer.Id, Type: model.TicketTypeRefund, Status: model.TicketStatusOpen, Subject: "Refund"}).Error)
	require.NoError(t, db.Create(&model.TopUp{Id: 10, UserId: viewer.Id, TradeNo: "PAY-A-001", Money: 100, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"a","account_name":"Account A"}`}).Error)
	require.NoError(t, db.Create(&model.TopUp{Id: 11, UserId: viewer.Id, TradeNo: "PAY-B-002", Money: 50, Status: common.TopUpStatusSuccess, EpayCredentials: `{"gateway_id":"b","account_name":"Account B"}`}).Error)
	require.NoError(t, db.Create(&model.TicketRefund{TicketId: 99, UserId: viewer.Id, RefundStatus: model.RefundStatusPending, RefundQuota: 50, FrozenQuota: 50}).Error)
	data := model.RefundExportData{TicketID: 99, CashCents: 5000, Invoices: []model.RefundExportInvoice{{InvoiceTicketID: 88, CompanyName: "=original", TaxNumber: "001234567890", Email: "original@example.com", InvoiceType: "special", VATRateBps: 1300, OrderIDs: []int{10, 11}, PaidCents: 15000, OriginalCents: 15000, RefundCents: 5000, ReissueCents: 10000}}}
	data.Payments = []model.RefundExportPayment{{OrderID: 10, RefundCents: 5000}}
	raw, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.RefundInvoiceExport{TicketID: 99, UserID: viewer.Id, CashCents: 5000, DataJSON: string(raw)}).Error)
	for _, tc := range []struct {
		role    int
		allowed bool
	}{{common.RoleCommonUser, false}, {common.RoleCustomerServiceUser, false}, {common.RoleFinanceUser, true}, {common.RoleAdminUser, true}, {common.RoleRootUser, true}} {
		t.Run(strconv.Itoa(tc.role), func(t *testing.T) {
			require.NoError(t, db.Model(&viewer).Update("role", tc.role).Error)
			listResponse := httptest.NewRecorder()
			listContext, _ := gin.CreateTestContext(listResponse)
			listContext.Set("id", viewer.Id)
			listContext.Set("role", common.RoleRootUser)
			listContext.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/admin/invoice/export-list?kind=refund", nil)
			GetInvoiceExportList(listContext)
			if tc.allowed {
				assert.Equal(t, http.StatusOK, listResponse.Code)
				assert.Contains(t, listResponse.Body.String(), `"invoice_color":"red"`)
				assert.Contains(t, listResponse.Body.String(), `"invoice_color":"blue"`)
				assert.NotContains(t, listResponse.Body.String(), "Account B")
				assert.NotContains(t, listResponse.Body.String(), "PAY-B-002")
				assert.Equal(t, "no-store", listResponse.Header().Get("Cache-Control"))
			} else {
				assert.Equal(t, http.StatusForbidden, listResponse.Code)
				assert.NotContains(t, listResponse.Body.String(), "original@example.com")
			}
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("id", viewer.Id)
			c.Set("role", common.RoleRootUser) // Never trust a stale session role.
			c.Params = gin.Params{{Key: "id", Value: "99"}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/refund/99/invoice-export?kind=reissue", nil)
			RefundInvoiceExport(c)
			if !tc.allowed {
				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.NotContains(t, response.Body.String(), "original@example.com")
				return
			}
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(response.Body.String(), "\xef\xbb\xbf"))).ReadAll()
			require.NoError(t, err)
			require.Len(t, rows, 2)
			assert.Equal(t, []string{"电子邮箱", "数量", "单价", "金额合计", "公司信息", "应税服务名称", "发票类型", "发票税率(%)", "开票方（公账名称）", "关联充值订单号"}, rows[0])
			assert.Equal(t, "original@example.com", rows[1][0])
			assert.Equal(t, "50.00", rows[1][3])
			assert.Equal(t, "发票抬头\n=original\n购方税号\n001234567890", rows[1][4])
			assert.Equal(t, "'=API service", rows[1][5])
			assert.Equal(t, "增值税普通发票", rows[1][6])
			assert.Equal(t, "6", rows[1][7])
			assert.Equal(t, "Account A", rows[1][8])
			assert.Equal(t, "PAY-A-001", rows[1][9])
		})
	}
	for _, kind := range []string{"red", "history"} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", viewer.Id)
		c.Params = gin.Params{{Key: "id", Value: "99"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/refund/99/invoice-export?kind="+kind, nil)
		RefundInvoiceExport(c)
		if kind == "history" {
			assert.Equal(t, http.StatusBadRequest, response.Code)
			continue
		}
		assert.Equal(t, http.StatusOK, response.Code)
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(response.Body.String(), "\xef\xbb\xbf"))).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "100.00", rows[1][3])
		assert.Equal(t, "Account A", rows[1][8])
		assert.Equal(t, "增值税专用发票", rows[1][6])
		assert.Equal(t, "13", rows[1][7])
		assert.Equal(t, "PAY-A-001", rows[1][9])
	}
	require.NoError(t, db.Model(&model.TopUp{}).Where("id = ?", 11).Update("user_id", viewer.Id+1).Error)
	items, err := model.RefundInvoiceExportItems(99, &data, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, []string{"PAY-A-001"}, items[0].OrderNumbers)
	assert.Equal(t, []model.InvoiceIssuerAmount{{AccountName: "Account A", AmountCents: 10000}}, items[0].IssuerAmounts)
}
