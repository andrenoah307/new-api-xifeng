package model

import "math"

// Invoice balances and wallet refunds are separate: retained unused cash must
// not become a replacement invoice merely because it was not refunded today.
func applyConsumedInvoiceAmounts(data *RefundExportData, orders []TopUp, lots map[int]RefundCreditLot) error {
	sources := make(map[int]TopUp, len(orders))
	refunded := map[int]int64{}
	for _, order := range orders {
		sources[order.Id] = order
	}
	for _, payment := range data.Payments {
		refunded[payment.OrderID] = payment.RefundCents
	}
	for i := range data.Invoices {
		invoice := &data.Invoices[i]
		used, retained := map[string]int64{}, map[string]int64{}
		affected, known := map[string]bool{}, map[string]bool{}
		for _, issuer := range invoice.IssuerAmounts {
			known[issuer.AccountName] = true
		}
		for _, id := range invoice.OrderIDs {
			order, exists := sources[id]
			if !exists {
				return ErrRefundExport
			}
			issuer, err := invoiceOrderIssuerName(order)
			if err != nil {
				return err
			}
			if len(known) == 1 && known["-"] {
				issuer = "-"
			}
			if !known[issuer] {
				return ErrRefundExport
			}
			lot := lots[id]
			cash := order.PaymentTotalCents - lot.RefundedCash - refunded[id]
			consumed := min(cash, refundProportionalCents(order.PaymentTotalCents, lot.UsedPaid, order.QuotaGranted))
			if cash < 0 || consumed < 0 || cash > math.MaxInt64-retained[issuer] || consumed > math.MaxInt64-used[issuer] {
				return ErrRefundExport
			}
			used[issuer] += consumed
			retained[issuer] += cash
			affected[issuer] = affected[issuer] || refunded[id] > 0
		}
		for _, original := range invoice.IssuerAmounts {
			name, amount := original.AccountName, original.AmountCents
			if affected[name] {
				amount = used[name]
				invoice.AffectedIssuers = append(invoice.AffectedIssuers, name)
			}
			if amount > math.MaxInt64-invoice.ReissueCents {
				return ErrRefundExport
			}
			invoice.ReissueCents += amount
			invoice.ReissueIssuerAmounts = append(invoice.ReissueIssuerAmounts, InvoiceIssuerAmount{AccountName: name, AmountCents: amount})
			invoice.UninvoicedIssuerAmounts = append(invoice.UninvoicedIssuerAmounts, InvoiceIssuerAmount{AccountName: name, AmountCents: max(int64(0), retained[name]-amount)})
		}
	}
	return nil
}
