package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportConfiguredChannelsAndFrozenRetry(t *testing.T) {
	costReportJobFixture(t)
	require.NoError(t, DB.Where("1 = 1").Delete(&SystemTask{}).Error)
	require.NoError(t, DB.AutoMigrate(&Channel{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&Channel{}).Error)
	t.Cleanup(func() { DB.Where("1 = 1").Delete(&Channel{}) })
	zero, ratio, peak, enabled := 0.0, 0.4, 1.5, true
	channels := []Channel{{Name: "zero", Key: "private-zero"}, {Name: "automatic", Key: "private-auto"}, {Name: "missing"}, {Name: "disabled"}, {Name: "changed-account", Key: "new-key"}}
	settings := []ChannelCostSettings{
		{Mode: "manual", Ratio: &zero},
		{Mode: "auto", Ratio: &ratio, Version: "v1", Upstream: &UpstreamCostDeclaration{PeakEnabled: &enabled, PeakStart: "09:00", PeakEnd: "18:00", PeakRate: &peak, Timezone: "Asia/Shanghai"}},
		{}, {Mode: "disabled", Ratio: &ratio}, {Mode: "auto", Ratio: &ratio, Identity: "old-account"},
	}
	for i := range channels {
		if i == 1 {
			settings[i].Identity = channels[i].CostIdentity()
		}
		encoded, err := common.Marshal(settings[i])
		require.NoError(t, err)
		channels[i].CostSettings = string(encoded)
		require.NoError(t, DB.Create(&channels[i]).Error)
	}
	ctx := context.Background()
	preview, err := PreviewChannelCostReportConfig(ctx, 0)
	require.NoError(t, err)
	require.Len(t, preview.Channels, 2)
	assert.Equal(t, 3, preview.Skipped)
	assert.Equal(t, "0", preview.Channels[0].Ratio)
	assert.Equal(t, "0.4", preview.Channels[1].Ratio)
	require.NotNil(t, preview.Channels[1].Peak)
	assert.Equal(t, "1.5", preview.Channels[1].Peak.Ratio)
	encoded, err := common.Marshal(preview)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-")
	assert.NotContains(t, string(encoded), settings[1].Identity)
	policy := CostReportPolicy{Start: 100, End: 3700, QuotaPerUnit: "1", FallbackMode: "billed_basis", CostFingerprint: preview.Fingerprint}
	key := uuid.NewString()
	job, err := CreateConfiguredChannelCostReportJob(ctx, 989, key, policy)
	require.NoError(t, err)
	var frozen CostReportPolicy
	require.NoError(t, common.UnmarshalJsonStr(job.PolicyJSON, &frozen))
	require.Len(t, frozen.Rates, 2)
	assert.Equal(t, preview.QuotaPerUnit, frozen.QuotaPerUnit, "conversion unit comes from the confirmed server snapshot")
	assert.Equal(t, preview.Channels[1].Peak, frozen.Rates[1].Peak)
	require.NoError(t, DB.Model(&channels[1]).Update("cost_settings", `{"mode":"manual","ratio":0.9}`).Error)
	_, err = CreateConfiguredChannelCostReportJob(ctx, 989, uuid.NewString(), policy)
	require.ErrorIs(t, err, ErrCostReportConfigChanged)
	retry, err := CreateConfiguredChannelCostReportJob(ctx, 989, key, policy)
	require.NoError(t, err)
	assert.Equal(t, job.ID, retry.ID)
	assert.Equal(t, job.PolicyJSON, retry.PolicyJSON)
	policy.Rates = []CostReportRate{{ChannelID: channels[0].Id, Ratio: "99"}}
	_, err = CreateConfiguredChannelCostReportJob(ctx, 989, uuid.NewString(), policy)
	require.Error(t, err, "client-supplied rates cannot override saved settings")
	onlyMissing, err := PreviewChannelCostReportConfig(ctx, channels[2].Id)
	require.NoError(t, err)
	assert.Empty(t, onlyMissing.Channels)
	assert.Equal(t, 1, onlyMissing.Skipped)
}
