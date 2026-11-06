package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type InvoiceIssuerAmount struct {
	AccountName string `json:"account_name"`
	AmountCents int64  `json:"amount_cents"`
}

func attachInvoiceIssuerAmounts(db *gorm.DB, items []*InvoiceExportItem, itemOrderIDs [][]int) error {
	ids := make([]int, 0)
	seen := map[int]bool{}
	for _, orderIDs := range itemOrderIDs {
		for _, id := range orderIDs {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	orders := map[int]TopUp{}
	for start := 0; start < len(ids); start += 500 {
		var batch []TopUp
		if err := db.Select("id", "user_id", "money", "status", "checkout_version", "payment_total_cents", "epay_credentials").Where("id IN ?", ids[start:min(start+500, len(ids))]).Find(&batch).Error; err != nil {
			return err
		}
		for _, order := range batch {
			orders[order.Id] = order
		}
	}
	for index, item := range items {
		total, err := invoiceMoneyCents(item.TotalMoney)
		if err != nil {
			return err
		}
		rows := make([]InvoiceIssuerAmount, 0)
		positions := map[string]int{}
		seenOrders := map[int]bool{}
		var attributed, orderTotal int64
		ambiguous := false
		for _, id := range itemOrderIDs[index] {
			if seenOrders[id] {
				continue
			}
			seenOrders[id] = true
			order, ok := orders[id]
			if !ok || order.UserId != item.UserId || order.Status != common.TopUpStatusSuccess {
				continue
			}
			amount, err := invoiceMoneyCents(order.Money)
			if order.CheckoutVersion == 1 {
				amount = order.PaymentTotalCents
			}
			if err != nil || amount <= 0 || amount > total-orderTotal {
				ambiguous = true
				break
			}
			orderTotal += amount
			var snapshot struct {
				GatewayID   string `json:"gateway_id"`
				AccountName string `json:"account_name"`
			}
			if order.EpayCredentials == "" {
				continue
			}
			if err := common.UnmarshalJsonStr(order.EpayCredentials, &snapshot); err != nil {
				return err
			}
			name := strings.TrimSpace(snapshot.AccountName)
			if snapshot.GatewayID == "" || snapshot.GatewayID == "private" || name == "" || name == "-" {
				continue
			}
			position, exists := positions[name]
			if !exists {
				position = len(rows)
				positions[name] = position
				rows = append(rows, InvoiceIssuerAmount{AccountName: name})
			}
			rows[position].AmountCents += amount
			attributed += amount
		}
		// Historical invoices have no per-order allocation snapshot. Never guess
		// how refunds or deductions reduced amounts across different issuers.
		if ambiguous {
			rows = nil
			attributed = 0
		}
		if attributed < total || len(rows) == 0 {
			rows = append(rows, InvoiceIssuerAmount{AccountName: "-", AmountCents: total - attributed})
		}
		item.IssuerAmounts = rows
	}
	return nil
}
