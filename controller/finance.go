package controller

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetFinancePolicy(c *gin.Context) {
	policy, err := model.GetFinancePolicy()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, policy)
}

func UpdateFinancePolicy(c *gin.Context) {
	var policy model.FinancePolicy
	if err := c.ShouldBindJSON(&policy); err != nil || !policy.Valid() {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.SaveFinancePolicy(policy); err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	recordManageAudit(c, "option.update", map[string]interface{}{"key": model.FinancePolicyKey, "value": policy})
	common.ApiSuccess(c, policy)
}

func GetFinancePolicyChoices(c *gin.Context) {
	choices, err := model.GetFinancePolicyChoices(c.Request.Context())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	common.ApiSuccess(c, choices)
}

func financeRequestFilter(c *gin.Context) (model.FinanceFilter, bool) {
	filter := model.FinanceFilter{View: c.DefaultQuery("view", "invoices"), Keyword: strings.TrimSpace(c.Query("keyword")), Status: c.Query("status")}
	if filter.View != "invoices" && filter.View != "topups" || len(filter.Keyword) > 100 {
		return filter, false
	}
	for key, target := range map[string]*int64{"start_time": &filter.StartTime, "end_time": &filter.EndTime} {
		if raw := c.Query(key); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				return filter, false
			}
			*target = value
		}
	}
	if filter.EndTime > 0 && filter.StartTime > filter.EndTime {
		return filter, false
	}
	if raw := c.Query("invoice_status"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || !model.IsValidInvoiceStatus(value) {
			return filter, false
		}
		filter.InvoiceStatus = value
	}
	return filter, true
}

func GetFinanceRecords(c *gin.Context) {
	filter, valid := financeRequestFilter(c)
	if !valid {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	policy, err := model.GetFinancePolicy()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	page := common.GetPageQuery(c)
	page.PageSize = min(page.PageSize, 100)
	if page.Page > 1000000 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	items, total, err := model.ListFinanceRecords(c.Request.Context(), filter, policy, page)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	c.Header("Cache-Control", "no-store")
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

// Protect spreadsheet imports as well as CSV syntax, including whitespace-prefixed formulas.
func financeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if strings.ContainsAny(value, "\t\r\n") || len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func ExportFinanceRecords(c *gin.Context) {
	filter, valid := financeRequestFilter(c)
	filter.SkipTotal = true
	if !valid {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	policy, err := model.GetFinancePolicy()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	// Finish the export before sending headers so query failures never become a
	// successful-looking, truncated CSV. The temporary file has mode 0600.
	file, err := os.CreateTemp("", "finance-export-*.csv")
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.WriteString("\xef\xbb\xbf"); err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	writer := csv.NewWriter(file)
	err = writer.Write([]string{"Record type", "Record ID", "User ID", "Username", "Ticket ID", "Invoice title", "Amount", "Invoice status", "Top-up status", "Created at (UTC)", "Related top-up IDs", "Related trade numbers", "Related amounts", "Payment methods", "Sources", "Incomplete associations"})
	count := 0
	for err == nil {
		if c.Request.Context().Err() != nil {
			return
		}
		var items []model.FinanceRecord
		items, _, err = model.ListFinanceRecords(c.Request.Context(), filter, policy, &common.PageInfo{Page: 1, PageSize: 500})
		if err != nil || len(items) == 0 {
			break
		}
		for _, item := range items {
			var ids, trades, amounts, methods, sources []string
			for _, order := range item.TopUps {
				ids = append(ids, strconv.Itoa(order.Id))
				trades = append(trades, order.TradeNo)
				amounts = append(amounts, strconv.FormatFloat(order.Money, 'f', 2, 64))
				methods = append(methods, order.PaymentMethod)
				sources = append(sources, order.Source)
			}
			row := []string{filter.View, strconv.Itoa(item.Id), strconv.Itoa(item.UserId), item.Username, strconv.Itoa(item.TicketId), item.CompanyName, strconv.FormatFloat(item.Money, 'f', 2, 64), strconv.Itoa(item.InvoiceStatus), item.Status, time.Unix(item.CreatedTime, 0).UTC().Format(time.RFC3339), strings.Join(ids, " | "), strings.Join(trades, " | "), strings.Join(amounts, " | "), strings.Join(methods, " | "), strings.Join(sources, " | "), strconv.FormatBool(item.IncompleteOrders)}
			for i := range row {
				row[i] = financeCSVCell(row[i])
			}
			if err = writer.Write(row); err != nil {
				break
			}
			count++
		}
		filter.BeforeID = items[len(items)-1].Id
	}
	writer.Flush()
	if err != nil || writer.Error() != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	current, err := model.GetFinancePolicy()
	if err != nil || !reflect.DeepEqual(current, policy) {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	if err = file.Close(); err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("X-Export-Count", strconv.Itoa(count))
	c.FileAttachment(file.Name(), fmt.Sprintf("finance_%s_%s.csv", filter.View, time.Now().UTC().Format("20060102_150405")))
}
