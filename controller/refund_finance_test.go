package controller

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundFinanceFreshRolesAndExport(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TicketRefund{}, &model.RefundOrderAllocation{}, &model.RefundInvoiceCorrection{}))
	user := model.User{Username: "refund-export", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled, AffCode: "refund-export"}
	require.NoError(t, db.Create(&user).Error)
	refund := model.TicketRefund{TicketId: 12, UserId: 99, FinanceVersion: 1, RefundCashCents: 530000, RefundStatus: model.RefundStatusPending}
	require.NoError(t, db.Create(&refund).Error)
	for _, id := range []int{1, 2} {
		require.NoError(t, db.Create(&model.RefundOrderAllocation{RefundTicketID: 12, TopUpID: id, UserID: 99, TradeNo: "=SUM(1,2)", InvoiceTicketID: 8, OriginalQuota: 2500000000, QuotaPerUnit: "500000", UsedQuota: 1250000000, RefundableQuota: 1250000000, RefundQuota: 1250000000, OriginalPaidCents: 530000, RefundCents: 265000, RefundTaxCents: 15000, UsedInvoiceCents: 265000, RemainingInvoiceCents: 265000, Status: "reserved"}).Error)
	}
	require.NoError(t, db.Create(&model.RefundInvoiceCorrection{UserId: 99, RefundTicketId: 12, InvoiceTicketId: 8, OriginalCents: 1060000, RefundCents: 530000, ReissueCents: 530000, Status: "pending"}).Error)
	for _, role := range []int{1, 3, 5, 10, 100} {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("role", role).Error)
		for _, handler := range []gin.HandlerFunc{ListRefundFinance, ExportRefundFinance, StartRefundInvoiceCorrection, CompleteRefundInvoiceCorrection} {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("id", user.Id)
			c.Set("role", 100)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/refund/finance-export?ticket_id=12", nil)
			handler(c)
			if role == 1 || role == 5 {
				assert.Equal(t, http.StatusForbidden, response.Code)
			} else {
				assert.NotEqual(t, http.StatusForbidden, response.Code)
			}
		}
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", user.Id)
	c.Request = httptest.NewRequest(http.MethodGet, "/?ticket_id=12", nil)
	ExportRefundFinance(c)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(response.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "'=SUM(1,2)", rows[1][2])
	assert.Equal(t, "5000.000000", rows[1][5])
	assert.Equal(t, "2500.000000", rows[1][6])
	assert.Equal(t, "2650.00", rows[1][11])
	assert.Equal(t, "10600.00", rows[1][15])
	assert.Equal(t, "5300.00", rows[1][16])
	assert.Empty(t, rows[2][15])
	assert.Empty(t, rows[2][16])
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)
	response = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(response)
	c.Set("id", user.Id)
	c.Request = httptest.NewRequest(http.MethodGet, "/?ticket_id=12", nil)
	ExportRefundFinance(c)
	assert.Equal(t, http.StatusForbidden, response.Code)
}
