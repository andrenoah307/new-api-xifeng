package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringCandidates(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Channel{}))
	channels := make([]Channel, 501)
	for i := range channels {
		channels[i] = Channel{Name: fmt.Sprintf("candidate-test-%02d", i), Key: "secret"}
	}
	require.NoError(t, DB.Create(&channels).Error)
	t.Cleanup(func() { DB.Where("name LIKE ?", "candidate-test-%").Delete(&Channel{}) })
	rows, err := MonitoringCandidates(false, "candidate-test-", nil, false)
	require.NoError(t, err)
	assert.Len(t, rows, 20)
	rows, err = MonitoringCandidates(false, "", []int{channels[0].Id, channels[24].Id}, true)
	require.NoError(t, err)
	assert.ElementsMatch(t, []MonitoringCandidate{{channels[0].Id, channels[0].Name}, {channels[24].Id, channels[24].Name}}, rows)
	ids := make([]int, len(channels))
	for i := range channels {
		ids[i] = channels[i].Id
	}
	rows, err = MonitoringCandidates(false, "", ids, true)
	require.NoError(t, err)
	assert.Len(t, rows, len(channels), "saved selections spanning multiple batches all retain their labels")
	rows, err = MonitoringCandidates(false, "candidate-test-%", nil, false)
	require.NoError(t, err)
	assert.Empty(t, rows, "wildcards are literal name characters")
	rows, err = MonitoringCandidates(false, "", nil, false)
	require.NoError(t, err)
	assert.Empty(t, rows, "empty search never loads the full table")
	user := User{Username: "candidate-login", DisplayName: "candidate-display", Password: "secret"}
	require.NoError(t, DB.Create(&user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&user) })
	rows, err = MonitoringCandidates(true, "candidate-display", nil, false)
	require.NoError(t, err)
	assert.Equal(t, []MonitoringCandidate{{user.Id, user.Username}}, rows)
}
