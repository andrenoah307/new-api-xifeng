package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingGroupVisibility(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "shared", EnableGroup: []string{"default", "private"}},
		{ModelName: "restricted", EnableGroup: []string{"private"}},
		{ModelName: "universal", EnableGroup: []string{"all", "private"}},
	}
	visible := filterPricingByUsableGroups(pricing, map[string]string{"default": "Default"})
	require.Len(t, visible, 2)
	assert.Equal(t, "shared", visible[0].ModelName)
	assert.Equal(t, []string{"default"}, visible[0].EnableGroup)
	assert.Equal(t, "universal", visible[1].ModelName)
	assert.Equal(t, []string{"all"}, visible[1].EnableGroup)
	assert.Empty(t, filterPricingByUsableGroups(pricing, nil))

	// Another user's request must still see the original groups in the cache.
	other := filterPricingByUsableGroups(pricing, map[string]string{"private": "Private"})
	require.Len(t, other, 3)
	assert.Equal(t, []string{"private"}, other[0].EnableGroup)
	assert.Equal(t, []string{"private"}, other[1].EnableGroup)
	assert.Equal(t, []string{"all", "private"}, other[2].EnableGroup)
	visible[0].EnableGroup[0] = "changed"
	assert.Equal(t, []string{"default", "private"}, pricing[0].EnableGroup)
	assert.Equal(t, []string{"all", "private"}, pricing[2].EnableGroup)
}
