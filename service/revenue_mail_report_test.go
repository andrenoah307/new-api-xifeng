package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueChineseTemplatePreview(t *testing.T) {
	c := model.DefaultRevenueAutomationConfig()
	c.Subject, c.Body = RevenueChineseMailSubject, RevenueChineseMailBody
	c.Fields = append(c.Fields, model.RevenueField{Key: "cost", Metric: "cost", Period: "day", Days: 1})
	subject, body, err := RevenueMailPreview(c)
	require.NoError(t, err)
	assert.Contains(t, subject, "收入统计报告")
	assert.Contains(t, body, "预览示例数据，非实际经营数据")
	assert.Equal(t, 1, strings.Count(body, "渠道利润分布"), "same window is rendered once")
	assert.Contains(t, body, "-15.000000")
	assert.Contains(t, body, "#ce4545")
	assert.NotContains(t, body, "{{")
	assert.NotContains(t, body, "<script")
}

func TestRevenueChannelReportKnownCostsAndLimits(t *testing.T) {
	rows := []model.ChannelCostReportRow{
		{ChannelID: 1, OriginalCount: 1, OriginalCost: "100", FallbackCost: "20", KnownRevenue: "150"},
		{ChannelID: 2, FallbackCount: 1, OriginalCost: "0", FallbackCost: "60", KnownRevenue: "45"},
		{ChannelID: 999, MissingCount: 1, OriginalCost: "0", FallbackCost: "0", KnownRevenue: "0", UnknownRevenue: "9999"},
	}
	body, err := RenderRevenueChannelReport(rows, model.CostQueryRange{Start: 1, End: 3601}, 0, "<unit>")
	require.NoError(t, err)
	assert.Contains(t, body, "195.000000")
	assert.Contains(t, body, "180.000000")
	assert.Contains(t, body, "15.000000")
	assert.Contains(t, body, "7.69%")
	assert.Contains(t, body, "&lt;unit&gt;")
	assert.NotContains(t, body, "#999")
	for id := 3; id <= 12; id++ {
		rows = append(rows, model.ChannelCostReportRow{ChannelID: id, OriginalCount: 1, OriginalCost: "0", FallbackCost: "0", KnownRevenue: "1"})
	}
	body, err = RenderRevenueChannelReport(rows, model.CostQueryRange{}, 0, "units")
	require.NoError(t, err)
	assert.Contains(t, body, "10 / 12")
	assert.Contains(t, body, "205.000000", "totals include channels omitted from detail")
	body, err = RenderRevenueChannelReport(nil, model.CostQueryRange{}, 7, "units")
	require.NoError(t, err)
	assert.Contains(t, body, "暂无成本已确定")
	assert.NotContains(t, body, "NaN")
	rows[0].KnownRevenue = "invalid"
	_, err = RenderRevenueChannelReport(rows, model.CostQueryRange{}, 0, "units")
	require.Error(t, err)
}
