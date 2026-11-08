package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type RefundCorrectionExportRow struct {
	InvoiceExportItem
	OrderNumbers []string `json:"order_numbers"`
	InvoiceColor string   `json:"invoice_color"`
}

type RefundCorrectionExportItem struct {
	InvoiceExportItem
	CorrectionRows []RefundCorrectionExportRow `json:"correction_rows"`
}

// ListRefundCorrectionsForExport reads saved cash allocations without recalculating
// balances or changing invoice/refund states. Unsaved or cancelled plans are excluded.
func ListRefundCorrectionsForExport(filter InvoiceExportFilter, page *common.PageInfo) ([]RefundCorrectionExportItem, int64, error) {
	var plans []struct {
		RefundInvoiceExport
		Status      int
		CreatedTime int64
	}
	query := DB.Table("refund_invoice_exports AS e").
		Select("e.*, t.status, t.created_time").
		Joins("JOIN tickets t ON t.id = e.ticket_id AND t.user_id = e.user_id").
		Joins("JOIN ticket_refunds r ON r.ticket_id = e.ticket_id AND r.user_id = e.user_id").
		Where("t.type = ? AND t.deleted_at IS NULL AND r.refund_status IN ?", TicketTypeRefund, []int{RefundStatusPending, RefundStatusRefunded})
	if filter.TicketStatus > 0 {
		query = query.Where("t.status = ?", filter.TicketStatus)
	}
	if filter.StartTime > 0 {
		query = query.Where("t.created_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("t.created_time <= ?", filter.EndTime)
	}
	if err := query.Order("t.id DESC").Find(&plans).Error; err != nil {
		return nil, 0, err
	}
	items := make([]RefundCorrectionExportItem, 0)
	var total int64
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	keywordAmount, amountErr := strconv.ParseFloat(keyword, 64)
	for _, plan := range plans {
		var data RefundExportData
		if err := common.UnmarshalJsonStr(plan.DataJSON, &data); err != nil {
			return nil, 0, err
		}
		invoices := make([]RefundExportInvoice, 0, len(data.Invoices))
		var names, emails []string
		orders := map[int]bool{}
		for _, invoice := range data.Invoices {
			if invoice.OriginalCents <= 0 || invoice.RefundCents <= 0 {
				continue
			}
			invoices = append(invoices, invoice)
			names = append(names, invoice.CompanyName)
			emails = append(emails, invoice.Email)
			for _, id := range invoice.OrderIDs {
				orders[id] = true
			}
		}
		if len(invoices) == 0 {
			continue
		}
		name, email := strings.Join(names, "; "), strings.Join(emails, "; ")
		if keyword != "" && !strings.Contains(strings.ToLower(name+" "+email), keyword) && keyword != strconv.Itoa(plan.TicketID) && (amountErr != nil || keywordAmount != float64(plan.CashCents)/100) {
			continue
		}
		total++
		if total <= int64(page.GetStartIdx()) || len(items) >= page.GetPageSize() {
			continue
		}
		data.Invoices = invoices
		red, err := RefundInvoiceExportItems(plan.TicketID, &data, false)
		if err != nil {
			return nil, 0, err
		}
		blue, err := RefundInvoiceExportItems(plan.TicketID, &data, true)
		if err != nil {
			return nil, 0, err
		}
		item := RefundCorrectionExportItem{InvoiceExportItem: InvoiceExportItem{TicketId: plan.TicketID, CompanyName: name, Email: email, TotalMoney: float64(plan.CashCents) / 100, OrderCount: len(orders), Status: plan.Status, CreatedTime: plan.CreatedTime}}
		for i := range red {
			for j, issuer := range red[i].IssuerAmounts {
				redRow, blueRow := red[i].InvoiceExportItem, blue[i].InvoiceExportItem
				redRow.IssuerAmounts = []InvoiceIssuerAmount{issuer}
				blueRow.IssuerAmounts = []InvoiceIssuerAmount{blue[i].IssuerAmounts[j]}
				redRow.TotalMoney = float64(issuer.AmountCents) / 100
				blueRow.TotalMoney = float64(blue[i].IssuerAmounts[j].AmountCents) / 100
				item.CorrectionRows = append(item.CorrectionRows,
					RefundCorrectionExportRow{InvoiceExportItem: redRow, OrderNumbers: red[i].OrderNumbers, InvoiceColor: "red"})
				if blue[i].IssuerAmounts[j].AmountCents > 0 {
					item.CorrectionRows = append(item.CorrectionRows,
						RefundCorrectionExportRow{InvoiceExportItem: blueRow, OrderNumbers: blue[i].OrderNumbers, InvoiceColor: "blue"})
				}
			}
		}
		items = append(items, item)
	}
	return items, total, nil
}
