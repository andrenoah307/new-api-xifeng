package service

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const RevenueChineseMailSubject = "{{system_name}} 收入统计报告 · {{scheduled_at}}"
const RevenueChineseMailBody = `<h1 style="font-size:22px;margin:0 0 16px">渠道收入与利润报告</h1><p>统计生成时间：{{generated_at}}（北京时间）</p>{{channel_reports}}<h2 style="font-size:18px">自定义指标</h2>{{summary_table}}<p style="color:#666;font-size:12px">仅统计成本已确定的消费，包含快照成本及成功补全成本。未知成本记录不计入消费、成本及利润；本报告不代表现金收入或全站净利润。</p>`

// Render only published aggregates. No source-log scans or client-side chart
// dependencies are needed to display these bars in an email client.
func RenderRevenueChannelReport(rows []model.ChannelCostReportRow, window model.CostQueryRange, channelID int, unit string) (string, error) {
	type item struct {
		id                    int
		cost, revenue, profit decimal.Decimal
	}
	items := make([]item, 0, len(rows))
	cost, revenue := decimal.Zero, decimal.Zero
	for _, row := range rows {
		if row.OriginalCount+row.FallbackCount == 0 {
			continue
		}
		original, err := decimal.NewFromString(row.OriginalCost)
		if err != nil {
			return "", err
		}
		fallback, err := decimal.NewFromString(row.FallbackCost)
		if err != nil {
			return "", err
		}
		consumption, err := decimal.NewFromString(row.KnownRevenue)
		if err != nil {
			return "", err
		}
		c := original.Add(fallback)
		cost, revenue = cost.Add(c), revenue.Add(consumption)
		items = append(items, item{row.ChannelID, c, consumption, consumption.Sub(c)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].profit.Abs().Equal(items[j].profit.Abs()) {
			return items[i].id < items[j].id
		}
		return items[i].profit.Abs().GreaterThan(items[j].profit.Abs())
	})
	scope := "全部渠道"
	if channelID > 0 {
		scope = fmt.Sprintf("渠道 #%d", channelID)
	}
	margin := "—"
	if !revenue.IsZero() {
		margin = revenue.Sub(cost).Div(revenue).Mul(decimal.NewFromInt(100)).StringFixed(2) + "%"
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<section style="margin:24px 0"><h2 style="font-size:18px">%s</h2><p style="font-size:12px;color:#666">%s 至 %s（北京时间，结束时刻不含）<br>金额单位：%s</p><table width="100%%" cellpadding="6" style="border-collapse:collapse;font-size:13px"><tr><th>消费</th><th>成本</th><th>利润</th><th>利润率</th></tr><tr align="center"><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr></table>`, scope, time.Unix(window.Start, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04"), time.Unix(window.End, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04"), html.EscapeString(unit), revenue.StringFixed(6), cost.StringFixed(6), revenue.Sub(cost).StringFixed(6), margin)
	if len(items) == 0 {
		return out.String() + "<p>暂无成本已确定的消费记录。</p></section>", nil
	}
	count := len(items)
	chart := items
	if len(chart) > 10 {
		chart = chart[:10]
	}
	fmt.Fprintf(&out, `<h3 style="font-size:15px">渠道利润分布</h3><p style="font-size:12px;color:#666">按利润绝对值排序，展示 %d / %d 个渠道；上方合计包含全部符合口径的渠道。绿色为盈利，红色为亏损，条长按绝对值比较。</p><table width="100%%" cellpadding="5" style="table-layout:fixed;font-size:12px">`, len(chart), count)
	max := items[0].profit.Abs()
	for _, row := range chart {
		width := "0"
		if !max.IsZero() {
			width = row.profit.Abs().Div(max).Mul(decimal.NewFromInt(100)).StringFixed(2)
		}
		color := "#16856b"
		if row.profit.IsNegative() {
			color = "#ce4545"
		}
		fmt.Fprintf(&out, `<tr><td width="22%%">渠道 #%d</td><td><table role="presentation" width="%s%%" cellpadding="0" cellspacing="0"><tr><td height="12" bgcolor="%s"></td></tr></table></td><td width="32%%" align="right" style="word-break:break-all;color:%s">%s</td></tr>`, row.id, width, color, color, row.profit.StringFixed(6))
	}
	if len(items) > 100 {
		items = items[:100]
	}
	fmt.Fprintf(&out, `</table><h3 style="font-size:15px">渠道明细（%d / %d）</h3><table width="100%%" cellpadding="5" style="table-layout:fixed;border-collapse:collapse;font-size:12px;word-break:break-all"><tr style="background:#eef1f3"><th>渠道</th><th>消费</th><th>成本</th><th>利润</th><th>利润率</th></tr>`, len(items), count)
	for _, row := range items {
		margin = "—"
		if !row.revenue.IsZero() {
			margin = row.profit.Div(row.revenue).Mul(decimal.NewFromInt(100)).StringFixed(2) + "%"
		}
		fmt.Fprintf(&out, `<tr align="right" style="border-bottom:1px solid #ddd"><td>#%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, row.id, row.revenue.StringFixed(6), row.cost.StringFixed(6), row.profit.StringFixed(6), margin)
	}
	return out.String() + "</table></section>", nil
}
