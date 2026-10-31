package controller

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
	"unicode"
)

// CSV exports share the report projection: no source-log query, float conversion,
// or invented profit for incomplete accounting. All timestamps are Unix seconds.
func channelCostReportCSV(job channelCostReportJobDTO, rows []channelCostReportRowDTO) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	header := []string{"task_id", "status", "start_timestamp", "end_timestamp", "aggregated_through", "channel_id", "channel_name", "consume_amount", "refund_amount", "net_amount", "original_cost", "fallback_cost", "cost", "known_profit", "profit", "profit_margin_ratio", "cost_ratio", "coverage_ratio", "consume_count", "original_count", "fallback_count", "missing_count", "stale_count", "refund_count"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		profit, margin, ratio := "", "", ""
		if row.Profit != nil {
			profit = *row.Profit
		}
		if row.ProfitMargin != nil {
			margin = *row.ProfitMargin
		}
		if row.CostRatio != nil {
			ratio = *row.CostRatio
		}
		record := []string{safeCostReportCSVText(job.ID), safeCostReportCSVText(job.Status), strconv.FormatInt(job.Start, 10), strconv.FormatInt(job.End, 10), strconv.FormatInt(job.Cursor, 10), strconv.Itoa(row.ChannelID), safeCostReportCSVText(row.ChannelName), row.ConsumeAmount, row.RefundAmount, row.NetAmount, row.OriginalCost, row.FallbackCost, row.Cost, row.KnownProfit, profit, margin, ratio, row.Coverage, strconv.FormatInt(row.ConsumeCount, 10), strconv.FormatInt(row.OriginalCount, 10), strconv.FormatInt(row.FallbackCount, 10), strconv.FormatInt(row.MissingCount, 10), strconv.FormatInt(row.StaleCount, 10), strconv.FormatInt(row.RefundCount, 10)}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func safeCostReportCSVText(value string) string {
	trimmed := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\ufeff' })
	if strings.ContainsAny(value, "\t\r\n") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
