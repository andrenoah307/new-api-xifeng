package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelSkipsOrphanAbilityAndSelectsRemainingCandidate(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
		require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	})

	priority := int64(10)
	weight := uint(10)
	channel := &Channel{
		Id:       980101,
		Name:     "available-channel",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create([]Ability{
		{
			Group:     "default",
			Model:     "orphan-fallback-model",
			ChannelId: 980102,
			Enabled:   true,
			Priority:  &priority,
			Weight:    100,
		},
		{
			Group:     "default",
			Model:     "orphan-fallback-model",
			ChannelId: channel.Id,
			Enabled:   true,
			Priority:  &priority,
			Weight:    weight,
		},
	}).Error)

	selected, err := GetChannel("default", "orphan-fallback-model", 0, "")

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
}
