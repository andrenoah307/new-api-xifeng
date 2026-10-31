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
	header := []string{"task_id", "status", "start_timestamp", "end_timestamp", "aggregated_through", "channel_id", "channel_name", "cost", "consume_amount", "profit", "profit_margin_ratio"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.OriginalCount+row.FallbackCount == 0 {
			continue
		}
		margin := ""
		if row.KnownProfitMargin != nil {
			margin = *row.KnownProfitMargin
		}
		record := []string{safeCostReportCSVText(job.ID), safeCostReportCSVText(job.Status), strconv.FormatInt(job.Start, 10), strconv.FormatInt(job.End, 10), strconv.FormatInt(job.Cursor, 10), strconv.Itoa(row.ChannelID), safeCostReportCSVText(row.ChannelName), row.Cost, row.KnownRevenue, row.KnownProfit, margin}
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
