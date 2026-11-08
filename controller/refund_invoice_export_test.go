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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundInvoiceExportRolesAndReissueIdentity(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TicketRefund{}, &model.RefundInvoiceExport{}))
	viewer := model.User{Username: "export-viewer", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "export-viewer"}
	require.NoError(t, db.Create(&viewer).Error)
	require.NoError(t, db.Create(&model.TicketRefund{TicketId: 99, UserId: viewer.Id, RefundStatus: model.RefundStatusPending, RefundQuota: 50, FrozenQuota: 50}).Error)
	data := model.RefundExportData{TicketID: 99, CashCents: 5000, Invoices: []model.RefundExportInvoice{{InvoiceTicketID: 88, CompanyName: "=original", TaxNumber: "001234567890", Email: "original@example.com", InvoiceType: "special", VATRateBps: 1300, OrderIDs: []int{10, 11}, PaidCents: 15000, OriginalCents: 15000, RefundCents: 5000, ReissueCents: 10000}}}
	raw, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.RefundInvoiceExport{TicketID: 99, UserID: viewer.Id, CashCents: 5000, DataJSON: string(raw)}).Error)
	for _, tc := range []struct {
		role    int
		allowed bool
	}{{common.RoleCommonUser, false}, {common.RoleFinanceUser, true}, {common.RoleAdminUser, true}, {common.RoleRootUser, true}} {
		t.Run(strconv.Itoa(tc.role), func(t *testing.T) {
			require.NoError(t, db.Model(&viewer).Update("role", tc.role).Error)
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
			assert.Equal(t, "'=original", rows[1][3])
			assert.Equal(t, "001234567890", rows[1][4])
			assert.Equal(t, "original@example.com", rows[1][5])
			assert.Equal(t, "增值税普通发票", rows[1][6])
			assert.Equal(t, "6.00%", rows[1][7])
			assert.Equal(t, "100.00", rows[1][8])
		})
	}
}
