package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRevenueAutomationReusesReportsAndNeverReplaysUncertainDelivery(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RevenueAutomation{}, &model.RevenueAutomationRun{}, &model.RevenueAutomationJob{}, &model.ChannelCostReportJob{}, &model.Channel{}))
	for _, table := range []any{&model.RevenueAutomation{}, &model.RevenueAutomationRun{}, &model.RevenueAutomationJob{}, &model.ChannelCostReportJob{}} {
		require.NoError(t, model.DB.Where("1 = 1").Delete(table).Error)
		table := table
		t.Cleanup(func() { model.DB.Where("1 = 1").Delete(table) })
	}
	root := model.User{Id: 989, Username: "automation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&root).Error)
	ctx := context.Background()
	channel := model.Channel{Id: 1, Name: "automation-cost", CostSettings: `{"mode":"manual","ratio":0.5}`}
	require.NoError(t, model.DB.Create(&channel).Error)
	t.Cleanup(func() { model.DB.Delete(&channel) })
	preview, err := model.PreviewChannelCostReportConfig(ctx, 0)
	require.NoError(t, err)
	c := model.DefaultRevenueAutomationConfig()
	c.Enabled = true
	c.Fingerprint = preview.Fingerprint
	c.Periods = nil
	c.MailEnabled = true
	c.Recipients = []string{"finance@example.com"}
	c.Fields = []model.RevenueField{{Key: "昨日利润", Metric: "profit", Period: "day"}, {Key: "margin", Metric: "profit_margin", Period: "day"}}
	c.Subject = "昨日利润 {{fields.昨日利润.value}}"
	c.Body = "{{fields.昨日利润.value}} {{summary_table}} {{ channel_reports }}"
	require.NoError(t, model.SaveRevenueAutomation(ctx, root.Id, 0, c))
	config, _, err := model.GetRevenueAutomation(ctx)
	require.NoError(t, err)
	at := time.Now().In(model.RevenueTimezone).AddDate(0, 0, -1)
	window, err := model.RevenueWindow("day", 1, at)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(config).Update("next_at", at.Unix()).Error)
	var policy model.CostReportPolicy
	require.NoError(t, common.UnmarshalJsonStr(config.PolicyJSON, &policy))
	policy.Start, policy.End = window.Start, window.End
	p, err := common.Marshal(policy)
	require.NoError(t, err)
	identity, err := model.ChannelCostReportSourceIdentity(ctx)
	require.NoError(t, err)
	job := model.ChannelCostReportJob{ID: "existing-report", RequestKey: "existing-report", OwnerID: root.Id, SourceIdentity: identity, PolicyJSON: string(p), RowsJSON: `[{"channel_id":1,"consume_count":2,"original_count":1,"missing_count":1,"consume_amount":"30","refund_amount":"0","original_cost":"25","fallback_cost":"0","known_revenue":"20","unknown_revenue":"10"}]`, Status: "completed", Cursor: window.End}
	require.NoError(t, model.DB.Create(&job).Error)
	require.NoError(t, RunRevenueAutomationPass(ctx))
	require.NoError(t, RunRevenueAutomationPass(ctx))
	var run model.RevenueAutomationRun
	require.NoError(t, model.DB.First(&run).Error)
	assert.Equal(t, "sending", run.Status)
	var data RevenueAutomationData
	require.NoError(t, common.UnmarshalJsonStr(run.Data, &data))
	assert.Equal(t, "-5.000000", data.Values["fields.昨日利润.value"])
	assert.Equal(t, "昨日利润 -5.000000", data.Subject)
	assert.Contains(t, data.Body, "昨日利润")
	assert.Contains(t, data.Body, "渠道利润分布")
	assert.Contains(t, data.Body, "#ce4545")
	assert.NotContains(t, data.Body, "{{ channel_reports }}")
	assert.NotContains(t, data.Body, "{{fields.")
	assert.Equal(t, "-25.00%", data.Values["fields.margin.value"])
	assert.Equal(t, "50.00%", data.Values["fields.昨日利润.coverage"])
	require.Len(t, data.Windows, 1)
	var jobs int64
	require.NoError(t, model.DB.Model(&model.ChannelCostReportJob{}).Count(&jobs).Error)
	assert.EqualValues(t, 1, jobs)
	require.Len(t, data.Deliveries, 1)
	data.Deliveries[0].Status = "sending"
	encoded, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&run).Update("data", string(encoded)).Error)
	require.NoError(t, RunRevenueAutomationPass(ctx))
	require.NoError(t, model.DB.First(&run, "id = ?", run.ID).Error)
	assert.Equal(t, "unknown", run.Status)
}

func TestRevenueMailTemplate(t *testing.T) {
	c := model.DefaultRevenueAutomationConfig()
	c.Subject = "{{system_name}}"
	c.Body = "{{system_name}} {{fields.yesterday_profit.value}}"
	subject, body, err := RenderRevenueMail(c, map[string]string{"system_name": "<site>", "fields.yesterday_profit.value": "-123.456789"})
	require.NoError(t, err)
	assert.Equal(t, "<site>", subject)
	assert.Contains(t, body, "&lt;site&gt; -123.456789")
	c.Body = "{{fields.missing.value}}"
	_, _, err = RevenueMailPreview(c)
	require.Error(t, err)
	c.Body = "{{system_name"
	_, _, err = RevenueMailPreview(c)
	require.Error(t, err)
	c.Body = "{{summary_table}}"
	_, body, err = RevenueMailPreview(c)
	require.NoError(t, err)
	assert.Contains(t, body, "<table")
	c.Subject = "{{system_name}}"
	_, _, err = RenderRevenueMail(c, map[string]string{"system_name": "name\r\nBcc: victim@example.com", "summary_table": ""})
	require.Error(t, err)
}

func TestRevenueMailChineseFieldPreview(t *testing.T) {
	c := model.DefaultRevenueAutomationConfig()
	c.Fields[0].Key = "昨日利润"
	c.Subject = "利润：{{fields.昨日利润.value}}"
	c.Body = "{{fields.昨日利润.value}} / {{fields.昨日利润.coverage}} {{summary_table}}"
	subject, body, err := RevenueMailPreview(c)
	require.NoError(t, err)
	assert.Equal(t, "利润：123.456789", subject)
	assert.Contains(t, body, "123.456789 / 95.00%")
	assert.Contains(t, body, "昨日利润")
	assert.NotContains(t, body, "{{fields.")
}
