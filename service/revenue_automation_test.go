package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueAutomationSnapshotsLatestPolicyForEachRun(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RevenueAutomation{}, &model.RevenueAutomationRun{}, &model.RevenueAutomationJob{}, &model.ChannelCostReportJob{}))
	for _, table := range []any{&model.RevenueAutomation{}, &model.RevenueAutomationRun{}, &model.RevenueAutomationJob{}, &model.ChannelCostReportJob{}} {
		require.NoError(t, model.DB.Where("1 = 1").Delete(table).Error)
		t.Cleanup(func() { model.DB.Where("1 = 1").Delete(table) })
	}
	originalUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = originalUnit })
	common.QuotaPerUnit = 500000
	ctx := context.Background()
	root := model.User{Username: "latest-policy-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&root).Error)
	channel := model.Channel{Id: 1, Name: "latest-policy", CostSettings: `{"mode":"manual","ratio":0.5}`}
	require.NoError(t, model.DB.Create(&channel).Error)
	preview, err := model.PreviewChannelCostReportConfig(ctx, 0)
	require.NoError(t, err)
	c := model.DefaultRevenueAutomationConfig()
	c.Enabled, c.MailEnabled, c.Fingerprint = true, false, preview.Fingerprint
	c.Periods = []string{"day"}
	require.NoError(t, model.SaveRevenueAutomation(ctx, root.Id, 0, c))
	at := time.Now().In(model.RevenueTimezone).AddDate(0, 0, -2)
	require.NoError(t, model.DB.Model(&model.RevenueAutomation{}).Where("id = 1").Update("next_at", at.Unix()).Error)
	common.QuotaPerUnit = 1000000
	require.NoError(t, model.DB.Model(&channel).Update("cost_settings", `{"mode":"manual","ratio":0.75}`).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 2, Name: "new-channel", CostSettings: `{"mode":"manual","ratio":0.25}`}).Error)

	for _, tc := range []struct {
		at         time.Time
		quota      int
		unit       string
		cost       string
		noFallback bool
	}{
		{at, 1000000, "1000000", "0.75", false},
		{at.AddDate(0, 0, 1), 2000000, "2000000", "0.9", false},
		{at.AddDate(0, 0, 2), 2000000, "2000000", "0", true},
	} {
		if tc.noFallback {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ?", []int{1, 2}).Update("cost_settings", `{"mode":"none"}`).Error)
		}
		window, err := model.RevenueWindow("day", 1, tc.at)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.Log{CreatedAt: window.Start + 60, Type: model.LogTypeConsume, ChannelId: 1, Quota: tc.quota, Other: `{"group_ratio":1}`}).Error)
		require.NoError(t, RunRevenueAutomationPass(ctx))
		var run model.RevenueAutomationRun
		require.NoError(t, model.DB.Where("scheduled_at = ?", tc.at.Unix()).First(&run).Error)
		require.Equal(t, "waiting", run.Status, run.Error)
		var data RevenueAutomationData
		require.NoError(t, common.UnmarshalJsonStr(run.Data, &data))
		require.NotNil(t, data.Policy)
		assert.Equal(t, tc.unit, data.Policy.QuotaPerUnit)
		if tc.noFallback {
			assert.Empty(t, data.Policy.Rates)
			assert.Equal(t, "none", data.Policy.FallbackMode)
		} else {
			require.Len(t, data.Policy.Rates, 2)
			assert.Equal(t, tc.cost, data.Policy.Rates[0].Ratio)
		}
		require.NotEmpty(t, data.JobID)

		// Changes after the run starts apply to the next run, including after a restart.
		common.QuotaPerUnit = 2000000
		require.NoError(t, model.DB.Model(&channel).Update("cost_settings", `{"mode":"manual","ratio":0.9}`).Error)
		require.NoError(t, RunRevenueAutomationPass(ctx))
		job, err := model.GetChannelCostReportJob(ctx, data.JobID)
		require.NoError(t, err)
		task, err := model.GetSystemTaskByTaskID(job.SystemTaskID)
		require.NoError(t, err)
		claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "policy-test", common.GetTimestamp()+60)
		require.NoError(t, err)
		require.True(t, ok)
		handler := channelCostReportHandler{aggregate: model.AggregateChannelCostWindow, skipEmpty: model.SkipChannelCostReportEmptyWindows}
		handler.Run(ctx, claimed, "policy-test")
		job, err = model.GetChannelCostReportJob(ctx, data.JobID)
		require.NoError(t, err)
		require.Equal(t, "completed", job.Status)
		var rows []model.ChannelCostReportRow
		require.NoError(t, common.UnmarshalJsonStr(job.RowsJSON, &rows))
		require.Len(t, rows, 1)
		if tc.noFallback {
			assert.Equal(t, "0", rows[0].KnownRevenue)
			assert.Equal(t, "1", rows[0].UnknownRevenue)
		} else {
			assert.Equal(t, "1", rows[0].KnownRevenue)
		}
		assert.Equal(t, tc.cost, rows[0].FallbackCost)
		require.NoError(t, RunRevenueAutomationPass(ctx))
		require.NoError(t, RunRevenueAutomationPass(ctx))
		require.NoError(t, model.DB.First(&run, "id = ?", run.ID).Error)
		assert.Equal(t, "completed", run.Status, run.Error)
	}
}

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
	c.MailPeriods = []string{"day", "week", "half_month"}
	c.MailPeriod = "year" // The new cadence list takes precedence over the legacy field.
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
	originalPolicy := data.Policy
	data.Policy = nil // Simulate an execution persisted before per-run policy snapshots.
	require.NoError(t, model.DB.Model(&channel).Update("cost_settings", `{"mode":"manual","ratio":0.9}`).Error)
	encoded, err := common.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&run).Update("data", string(encoded)).Error)
	require.NoError(t, RunRevenueAutomationPass(ctx))
	require.NoError(t, model.DB.First(&run, "id = ?", run.ID).Error)
	assert.Equal(t, "unknown", run.Status)
	require.NoError(t, common.UnmarshalJsonStr(run.Data, &data))
	assert.Equal(t, originalPolicy, data.Policy)
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
