package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVisibleUserGroupsAppliesRatioAndRegionFilters(t *testing.T) {
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	previousRegion := operation_setting.GetRegionRestrictionSetting()
	defer func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(previousRatios)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsable)
		blockedJSON, _ := common.Marshal(previousRegion.BlockedGroups)
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"region_restriction.enabled":        fmt.Sprintf("%t", previousRegion.Enabled),
			"region_restriction.filter_console": fmt.Sprintf("%t", previousRegion.FilterConsole),
			"region_restriction.blocked_groups": string(blockedJSON),
		})
		operation_setting.RebuildRegionRestrictionIndex()
	}()

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"visible":1,"blocked":1,"not-usable":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"visible":"Visible","blocked":"Blocked"}`))
	blockedJSON, err := common.Marshal(map[string][]string{"CN": {"blocked"}})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"region_restriction.enabled":        "true",
		"region_restriction.filter_console": "true",
		"region_restriction.blocked_groups": string(blockedJSON),
	}))
	operation_setting.RebuildRegionRestrictionIndex()

	visible := GetVisibleUserGroups("member", "CN")
	assert.Contains(t, visible, "visible")
	assert.NotContains(t, visible, "blocked")
	assert.NotContains(t, visible, "not-usable")
}
