package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueCalendarWindows(t *testing.T) {
	for _, tc := range []struct {
		at, period, start, end string
		days                   int
	}{
		{"2024-03-01", "month", "2024-02-01", "2024-03-01", 1},
		{"2026-01-01", "week", "2025-12-22", "2025-12-29", 1},
		{"2026-01-01", "year", "2025-01-01", "2026-01-01", 1},
		{"2026-01-01", "quarter", "2025-10-01", "2026-01-01", 1},
		{"2026-11-01", "days", "2026-10-02", "2026-11-01", 30},
	} {
		t.Run(tc.at+tc.period, func(t *testing.T) {
			at, err := time.ParseInLocation("2006-01-02", tc.at, RevenueTimezone)
			require.NoError(t, err)
			window, err := RevenueWindow(tc.period, tc.days, at.UTC())
			require.NoError(t, err)
			assert.Equal(t, tc.start, time.Unix(window.Start, 0).In(RevenueTimezone).Format("2006-01-02"))
			assert.Equal(t, tc.end, time.Unix(window.End, 0).In(RevenueTimezone).Format("2006-01-02"))
		})
	}
	_, err := RevenueWindow("days", 367, time.Now())
	require.Error(t, err)
	_, err = RevenueWindow("days", 0, time.Now())
	require.Error(t, err)
	at, err := time.Parse(time.RFC3339, "2026-11-01T20:00:00Z")
	require.NoError(t, err)
	assert.True(t, RevenuePeriodDue("week", at))
	assert.False(t, RevenuePeriodDue("month", at))
	assert.Equal(t, "2026-11-03T04:00:00+08:00", time.Unix(RevenueNextDue("04:00", at), 0).In(RevenueTimezone).Format(time.RFC3339))
}

func TestRevenueConfigurationValidation(t *testing.T) {
	c := DefaultRevenueAutomationConfig()
	require.NoError(t, ValidateRevenueAutomationConfig(c))
	assert.False(t, c.Enabled)
	c.MailEnabled = true
	require.Error(t, ValidateRevenueAutomationConfig(c))
	c.Recipients = []string{"finance@example.com"}
	require.NoError(t, ValidateRevenueAutomationConfig(c))
	c.Fields = append(c.Fields, c.Fields[0])
	require.Error(t, ValidateRevenueAutomationConfig(c))
	c.Fields = c.Fields[:1]
	c.Recipients = []string{"finance@example.com\r\nBcc: leak@example.com"}
	require.Error(t, ValidateRevenueAutomationConfig(c))
}

func TestRevenueFieldKeys(t *testing.T) {
	for _, tc := range []struct {
		key   string
		valid bool
	}{
		{"昨日利润", true},
		{"渠道A_利润2026", true},
		{"yesterday_profit", true},
		{strings.Repeat("收", 48), true},
		{strings.Repeat("收", 49), false},
		{"", false},
		{"1收入", false},
		{"昨日 利润", false},
		{"收入.value", false},
		{"{{收入}}", false},
		{"收入\r\n", false},
		{"<收入>", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			c := DefaultRevenueAutomationConfig()
			c.Fields[0].Key = tc.key
			err := ValidateRevenueAutomationConfig(c)
			if tc.valid {
				require.NoError(t, err)
				c.Fields = append(c.Fields, c.Fields[0])
				require.EqualError(t, ValidateRevenueAutomationConfig(c), "Invalid field key")
			} else {
				require.EqualError(t, err, "Invalid field key")
			}
		})
	}
}

func TestRevenueAutomationJobCreationIsIdempotentAndWaitsForManualJob(t *testing.T) {
	manual, task := costReportJobFixture(t)
	require.NoError(t, DB.AutoMigrate(&RevenueAutomation{}, &RevenueAutomationRun{}, &RevenueAutomationJob{}, &Channel{}))
	for _, table := range []any{&RevenueAutomation{}, &RevenueAutomationRun{}, &RevenueAutomationJob{}, &Channel{}} {
		require.NoError(t, DB.Where("1 = 1").Delete(table).Error)
		table := table
		t.Cleanup(func() { DB.Where("1 = 1").Delete(table) })
	}
	channel := Channel{Name: "scheduled-cost", CostSettings: `{"mode":"manual","ratio":0.5}`}
	require.NoError(t, DB.Create(&channel).Error)
	ctx := context.Background()
	preview, err := PreviewChannelCostReportConfig(ctx, 0)
	require.NoError(t, err)
	c := DefaultRevenueAutomationConfig()
	c.Enabled = true
	c.Fingerprint = preview.Fingerprint
	require.NoError(t, SaveRevenueAutomation(ctx, 989, 0, c))
	config, _, err := ClaimRevenueAutomation(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)
	defer ReleaseRevenueAutomation(ctx, config)
	var policy CostReportPolicy
	require.NoError(t, common.UnmarshalJsonStr(config.PolicyJSON, &policy))
	policy.Start, policy.End = 100, 7300
	_, err = EnsureRevenueAutomationReport(ctx, config, policy)
	require.ErrorIs(t, err, ErrChannelCostReportConflict)
	var ownership int64
	require.NoError(t, DB.Model(&RevenueAutomationJob{}).Count(&ownership).Error)
	assert.Zero(t, ownership)
	var preserved ChannelCostReportJob
	require.NoError(t, DB.First(&preserved, "id = ?", manual.ID).Error)
	assert.Equal(t, "pending", preserved.Status)
	// Release only the fixture's manual executor, then repeat the same scheduler request.
	require.NoError(t, DB.Model(task).Updates(map[string]any{"status": SystemTaskStatusSucceeded, "active_key": nil}).Error)
	job, err := EnsureRevenueAutomationReport(ctx, config, policy)
	require.NoError(t, err)
	again, err := EnsureRevenueAutomationReport(ctx, config, policy)
	require.NoError(t, err)
	assert.Equal(t, job.ID, again.ID)
	assert.Equal(t, job.SystemTaskID, again.SystemTaskID)
	require.NoError(t, DB.Model(&RevenueAutomationJob{}).Count(&ownership).Error)
	assert.EqualValues(t, 1, ownership)
	ReleaseRevenueAutomation(ctx, config)
	sending := RevenueAutomationRun{ID: "disable-sending", ScheduledAt: 200, Status: "sending", Data: "{}"}
	require.NoError(t, DB.Create(&sending).Error)
	c.Enabled = false
	require.NoError(t, SaveRevenueAutomation(ctx, 989, 1, c))
	require.NoError(t, DB.First(&sending, "id = ?", sending.ID).Error)
	assert.Equal(t, "unknown", sending.Status)
	control, settings, err := ClaimRevenueAutomationControl(ctx)
	require.NoError(t, err)
	require.NotNil(t, control)
	assert.False(t, settings.Enabled)
	ReleaseRevenueAutomation(ctx, control)
}

func TestRevenueAutomationCASAndDisabledDefault(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&RevenueAutomation{}, &RevenueAutomationRun{}, &RevenueAutomationJob{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&RevenueAutomation{}).Error)
	require.NoError(t, DB.Where("1 = 1").Delete(&RevenueAutomationRun{}).Error)
	t.Cleanup(func() {
		DB.Where("1 = 1").Delete(&RevenueAutomation{})
		DB.Where("1 = 1").Delete(&RevenueAutomationRun{})
	})
	root := User{Username: "revenue-automation-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&root).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&root) })
	ctx := context.Background()
	row, c, err := GetRevenueAutomation(ctx)
	require.NoError(t, err)
	assert.Zero(t, row.Revision)
	assert.False(t, c.Enabled)
	claimed, _, err := ClaimRevenueAutomation(ctx)
	require.NoError(t, err)
	assert.Nil(t, claimed)
	require.NoError(t, SaveRevenueAutomation(ctx, root.Id, 0, c))
	assert.ErrorIs(t, SaveRevenueAutomation(ctx, root.Id, 0, c), ErrRevenueAutomationConflict)
	preview, err := PreviewChannelCostReportConfig(ctx, 0)
	if err == nil && len(preview.Channels) == 0 {
		channel := Channel{Name: "automation-cost", CostSettings: `{"mode":"manual","ratio":0.5}`}
		require.NoError(t, DB.Create(&channel).Error)
		t.Cleanup(func() { DB.Delete(&channel) })
		preview, err = PreviewChannelCostReportConfig(ctx, 0)
	}
	require.NoError(t, err)
	c.Enabled = true
	c.Fingerprint = preview.Fingerprint
	require.NoError(t, SaveRevenueAutomation(ctx, root.Id, 1, c))
	claimed, _, err = ClaimRevenueAutomation(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	second, _, err := ClaimRevenueAutomation(ctx)
	require.NoError(t, err)
	assert.Nil(t, second)
	assert.ErrorIs(t, SaveRevenueAutomation(ctx, root.Id, 2, c), ErrRevenueAutomationConflict)
	ReleaseRevenueAutomation(ctx, claimed)
	assert.ErrorIs(t, SaveRevenueAutomationRun(ctx, claimed, &RevenueAutomationRun{ID: "stale"}), ErrRevenueAutomationConflict)
	claimed, _, err = ClaimRevenueAutomation(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	testRevenueAutomationScheduling(t, claimed)
	ReleaseRevenueAutomation(ctx, claimed)
	require.NoError(t, DB.Model(&root).Update("role", common.RoleAdminUser).Error)
	_, _, err = ClaimRevenueAutomation(ctx)
	require.Error(t, err)
}

// Exercise the same scheduling contract on SQLite, MySQL and PostgreSQL.
func testRevenueAutomationScheduling(t *testing.T, claimed *RevenueAutomation) {
	t.Helper()
	ctx := context.Background()
	due := time.Now().Add(-time.Minute).Unix()
	claimed.NextAt = due
	require.NoError(t, DB.Model(&RevenueAutomation{}).Where("id = ?", claimed.ID).Update("next_at", due).Error)
	run, err := NextRevenueAutomationRun(ctx, claimed, "{}")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, due, run.ScheduledAt)
	assert.Equal(t, "pending", run.Status)
	again, err := NextRevenueAutomationRun(ctx, claimed, "{}")
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, run.ID, again.ID)
	var config RevenueAutomation
	require.NoError(t, DB.First(&config, claimed.ID).Error)
	assert.Equal(t, due+86400, config.NextAt)
	require.NoError(t, DB.Delete(run).Error)
	require.NoError(t, DB.Model(&RevenueAutomation{}).Where("id = ?", claimed.ID).Update("next_at", due).Error)
	// More than one cleanup batch: an unknown delivery must roll back prior deletes.
	oldRuns := make([]RevenueAutomationRun, 257)
	for i := range oldRuns {
		oldRuns[i] = RevenueAutomationRun{ID: fmt.Sprintf("history-%d", i), ScheduledAt: int64(i + 1), Status: "completed", Data: "{}"}
	}
	oldRuns[0].Status = "unknown"
	require.NoError(t, DB.Create(&oldRuns).Error)
	_, err = NextRevenueAutomationRun(ctx, claimed, "{}")
	require.ErrorContains(t, err, "uncertain")
	var count int64
	require.NoError(t, DB.Model(&RevenueAutomationRun{}).Count(&count).Error)
	assert.EqualValues(t, len(oldRuns), count)
	require.NoError(t, DB.First(&config, claimed.ID).Error)
	assert.Equal(t, due, config.NextAt)
	require.NoError(t, DB.Model(&oldRuns[0]).Update("status", "cancelled").Error)
	run, err = NextRevenueAutomationRun(ctx, claimed, "{}")
	require.NoError(t, err)
	require.NotNil(t, run)
	require.NoError(t, DB.Model(&RevenueAutomationRun{}).Count(&count).Error)
	assert.EqualValues(t, 128, count)
	var retained []RevenueAutomationRun
	require.NoError(t, DB.Where("id <> ?", run.ID).Order("scheduled_at ASC").Find(&retained).Error)
	require.Len(t, retained, 127)
	assert.Equal(t, oldRuns[130].ID, retained[0].ID)
	assert.Equal(t, oldRuns[256].ID, retained[126].ID)
	require.NoError(t, DB.First(&config, claimed.ID).Error)
	assert.Equal(t, due+86400, config.NextAt)
	run.Status = "completed"
	require.NoError(t, SaveRevenueAutomationRun(ctx, claimed, run))
}
