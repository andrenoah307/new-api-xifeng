package model

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type InvoiceGroupCloseItem struct {
	TicketID           int    `json:"ticket_id"`
	UserID             int    `json:"user_id"`
	SupplementTaxCents int64  `json:"supplement_tax_cents"`
	OrderTaxCents      int64  `json:"order_tax_cents"`
	QuotaDelta         string `json:"quota_delta"`
	Revision           string `json:"revision"`
	Error              string `json:"error,omitempty"`
	Applied            bool   `json:"applied"`
}

type InvoiceGroupClosePage struct {
	Items   []InvoiceGroupCloseItem `json:"items"`
	AfterID int                     `json:"after_id"`
	HasMore bool                    `json:"has_more"`
}

func PreviewInvoiceGroupClosures(actorID, afterID int, convert bool) (*InvoiceGroupClosePage, error) {
	config, err := GetInvoiceTaxConfig(DB)
	if err != nil {
		return nil, err
	}
	if !config.InvoiceRestrictedGroupsBlockTickets || len(config.InvoiceRestrictedGroups) == 0 {
		return nil, ErrTicketForbidden
	}
	var actor User
	if err := DB.First(&actor, actorID).Error; err != nil || actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
		return nil, ErrTicketForbidden
	}
	users := DB.Model(&User{}).Select("id").Where(commonGroupCol+" IN ?", config.InvoiceRestrictedGroups)
	var tickets []Ticket
	if err := DB.Where("id > ? AND type = ? AND status IN ? AND user_id IN (?)", afterID, TicketTypeInvoice, []int{TicketStatusOpen, TicketStatusProcessing}, users).Order("id ASC").Limit(201).Find(&tickets).Error; err != nil {
		return nil, err
	}
	page := &InvoiceGroupClosePage{Items: []InvoiceGroupCloseItem{}, AfterID: afterID, HasMore: len(tickets) > 200}
	if page.HasMore {
		tickets = tickets[:200]
	}
	for _, ticket := range tickets {
		item, err := CloseRestrictedInvoiceTicket(actorID, ticket.Id, convert, "", false)
		if err != nil {
			item = &InvoiceGroupCloseItem{TicketID: ticket.Id, UserID: ticket.UserId, Error: err.Error()}
		}
		page.Items = append(page.Items, *item)
		page.AfterID = ticket.Id
	}
	return page, nil
}

// Each ticket commits closure, tax returns and order snapshots together. A failed
// item stays unchanged; retries cannot credit a closed ticket a second time.
func CloseRestrictedInvoiceTicket(actorID, ticketID int, convert bool, revision string, apply bool) (*InvoiceGroupCloseItem, error) {
	var identity Ticket
	if err := DB.Select("id,user_id").First(&identity, ticketID).Error; err != nil {
		return nil, err
	}
	item := &InvoiceGroupCloseItem{TicketID: ticketID, UserID: identity.UserId}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner, actor User
		if err := lockForUpdate(tx).First(&owner, identity.UserId).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&actor, actorID).Error; err != nil || actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled {
			return ErrTicketForbidden
		}
		var option Option
		if err := lockForUpdate(tx).Where(Option{Key: InvoiceTaxConfigKey}).Take(&option).Error; err != nil {
			return err
		}
		config, err := GetInvoiceTaxConfig(tx)
		if err != nil {
			return err
		}
		if !config.InvoiceRestrictedGroupsBlockTickets || !config.RestrictsInvoiceGroup(owner.Group) {
			return ErrTicketForbidden
		}
		var ticket Ticket
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", ticketID, owner.Id).First(&ticket).Error; err != nil {
			return err
		}
		if ticket.Type != TicketTypeInvoice || (ticket.Status != TicketStatusOpen && ticket.Status != TicketStatusProcessing) {
			return ErrTicketClosed
		}
		var refunds int64
		if err := tx.Model(&TicketRefund{}).Where("user_id = ? AND refund_status = ?", owner.Id, RefundStatusPending).Count(&refunds).Error; err != nil {
			return err
		}
		if refunds != 0 {
			return ErrRefundFinancePending
		}
		var corrections int64
		if err := tx.Model(&RefundInvoiceCorrection{}).Where("invoice_ticket_id = ? AND status IN ?", ticketID, []string{"pending", "processing", "completed"}).Count(&corrections).Error; err != nil {
			return err
		}
		if corrections != 0 {
			return ErrRefundInvoiceCorrection
		}
		var invoice TicketInvoice
		invoiceErr := lockForUpdate(tx).Where("ticket_id = ? AND user_id = ?", ticketID, owner.Id).Take(&invoice).Error
		if invoiceErr != nil && !errors.Is(invoiceErr, gorm.ErrRecordNotFound) {
			return invoiceErr
		}
		if invoiceErr == nil && invoice.InvoiceStatus != InvoiceStatusPending {
			return ErrTicketInvoiceStatusInvalid
		}
		var charge InvoiceTaxCharge
		chargeErr := lockForUpdate(tx).Where("ticket_id = ?", ticketID).Take(&charge).Error
		if chargeErr != nil && !errors.Is(chargeErr, gorm.ErrRecordNotFound) {
			return chargeErr
		}
		if chargeErr == nil && (invoiceErr != nil || charge.UserId != owner.Id) {
			return ErrInvoiceTaxInvalid
		}
		quota := decimal.Zero
		if chargeErr == nil && charge.Status == "paid" {
			if charge.Quota < 0 || charge.FeeCents < 0 {
				return ErrInvoiceTaxInvalid
			}
			item.SupplementTaxCents = charge.FeeCents
			quota = decimal.NewFromInt(int64(charge.Quota))
		}
		var orders []*TopUp
		if invoiceErr == nil && convert {
			ids, err := invoice.GetTopUpOrderIDs()
			if err != nil {
				return err
			}
			orders, err = fetchSuccessTopUpsByIds(tx, owner.Id, ids)
			if err != nil {
				return err
			}
			if len(orders) != len(ids) {
				return ErrTicketInvoiceOrderInvalid
			}
			orders = orderTopUps(ids, orders)
			for _, order := range orders {
				if order.InvoiceType != "ordinary" && order.InvoiceType != "special" {
					continue
				}
				// Legacy explicit grants contain no prepaid tax to convert.
				if order.CheckoutVersion == 0 && order.InvoiceTaxCents == 0 {
					continue
				}
				plan, _, err := manageTopUpInvoiceType(tx, actorID, TopUpInvoiceAdjustmentInput{ID: order.Id, InvoiceType: "none", ExpectedInvoiceType: &order.InvoiceType}, false, ticketID)
				if err != nil {
					return err
				}
				delta, err := decimal.NewFromString(plan.QuotaDelta)
				if err != nil || delta.IsNegative() || !plan.CanApply {
					return ErrInvoiceAdjustment
				}
				quota = quota.Add(delta)
				item.OrderTaxCents += plan.TaxBeforeCents
			}
		}
		item.QuotaDelta = quota.String()
		// Balance is deliberately excluded: other tickets for this user may have
		// credited it since preview. Credit amounts and all source records are pinned.
		raw, err := common.Marshal(struct {
			Ticket  Ticket
			Invoice TicketInvoice
			Charge  InvoiceTaxCharge
			Orders  []*TopUp
			Config  InvoiceTaxConfig
			Group   string
			Convert bool
			Delta   string
			Unit    float64
		}{ticket, invoice, charge, orders, config, owner.Group, convert, item.QuotaDelta, common.QuotaPerUnit})
		if err != nil {
			return err
		}
		item.Revision = fmt.Sprintf("%x", sha256.Sum256(raw))
		if !apply {
			return nil
		}
		if revision != item.Revision {
			return ErrFinanceRevision
		}
		if invoiceErr == nil {
			if _, err := cancelPendingInvoice(tx, &ticket, actorID); err != nil {
				return err
			}
		}
		for _, order := range orders {
			if order.InvoiceType != "ordinary" && order.InvoiceType != "special" {
				continue
			}
			if order.CheckoutVersion == 0 && order.InvoiceTaxCents == 0 {
				continue
			}
			input := TopUpInvoiceAdjustmentInput{ID: order.Id, InvoiceType: "none", ExpectedInvoiceType: &order.InvoiceType}
			plan, _, err := manageTopUpInvoiceType(tx, actorID, input, false, 0)
			if err != nil {
				return err
			}
			input.Revision = plan.Revision
			if _, _, err := manageTopUpInvoiceType(tx, actorID, input, true, 0); err != nil {
				return err
			}
		}
		result := tx.Model(&Ticket{}).Where("id = ? AND user_id = ? AND status = ?", ticketID, owner.Id, ticket.Status).Updates(map[string]interface{}{"status": TicketStatusClosed, "admin_id": actorID, "updated_time": common.GetTimestamp()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrFinanceRevision
		}
		if err := recordTicketOperationTx(tx, ticketID, actorID, fmt.Sprintf("Restricted-group invoice closure: convert order tax=%t; supplement tax=%d cents; order tax=%d cents; balance credit=%s quota; revision=%s", convert, item.SupplementTaxCents, item.OrderTaxCents, item.QuotaDelta, item.Revision)); err != nil {
			return err
		}
		item.Applied = true
		return nil
	})
	if err == nil && item.Applied {
		if cacheErr := InvalidateUserCache(item.UserID); cacheErr != nil {
			common.SysError("invoice closure cache invalidation: " + cacheErr.Error())
		}
		RecordLogWithAdminInfo(item.UserID, LogTypeManage, "Invoice application closed; invoice fees returned to balance", map[string]interface{}{"admin_id": actorID, "ticket_id": ticketID, "quota_delta": item.QuotaDelta, "revision": item.Revision})
	}
	return item, err
}
