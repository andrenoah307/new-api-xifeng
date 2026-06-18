package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 模拟“资料更新整行覆盖并发扣费”：扣费用原子自减把库里 quota 改为 90，
// 随后用携带旧 quota=100 的快照调用 Update 改 DisplayName，断言 quota 不被覆盖。
func TestUserUpdate_DoesNotClobberAtomicColumns(t *testing.T) {
	truncateTables(t)

	u := &User{Username: "victim_lu", Password: "hashed", DisplayName: "old", Quota: 100, UsedQuota: 0, RequestCount: 0}
	require.NoError(t, DB.Create(u).Error)

	// 并发扣费（原子）：quota 100->90, used_quota 0->5, request_count 0->1
	require.NoError(t, decreaseUserQuota(u.Id, 10))
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Update("used_quota", 5).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Update("request_count", 1).Error)

	// 旧快照资料更新
	stale := &User{Id: u.Id, Username: "victim_lu", DisplayName: "new", Quota: 100, UsedQuota: 0, RequestCount: 0}
	require.NoError(t, stale.Update(false))

	var got User
	require.NoError(t, DB.First(&got, u.Id).Error)
	assert.Equal(t, 90, got.Quota, "quota 不应被资料更新覆盖")
	assert.Equal(t, 5, got.UsedQuota, "used_quota 不应被覆盖")
	assert.Equal(t, 1, got.RequestCount, "request_count 不应被覆盖")
	assert.Equal(t, "new", got.DisplayName, "常规字段应正常更新")
}

// 管理员渠道编辑携带旧 used_quota 快照不得覆盖原子自增的统计值。
func TestChannelUpdate_DoesNotClobberUsedQuota(t *testing.T) {
	truncateTables(t)

	ch := &Channel{Name: "c-lu", Status: 1, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 1.5}
	require.NoError(t, DB.Create(ch).Error)

	// 并发原子自增 used_quota 0->50
	updateChannelUsedQuota(ch.Id, 50)

	// 旧快照编辑：改名，携带 used_quota=0 / balance=0
	stale := &Channel{Id: ch.Id, Name: "c-lu-2", Status: 1, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 0}
	require.NoError(t, stale.Update())

	var got Channel
	require.NoError(t, DB.First(&got, ch.Id).Error)
	assert.Equal(t, int64(50), got.UsedQuota, "used_quota 不应被渠道编辑覆盖")
	assert.Equal(t, "c-lu-2", got.Name, "常规字段应正常更新")
}

// SaveWithoutKey（自动封禁/压力冷却/中继报错路径）整行 Save 不得覆盖原子 used_quota。
func TestChannelSaveWithoutKey_DoesNotClobberUsedQuota(t *testing.T) {
	truncateTables(t)

	ch := &Channel{Name: "c-swk", Status: 1, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 2.0}
	require.NoError(t, DB.Create(ch).Error)

	updateChannelUsedQuota(ch.Id, 70)

	stale := &Channel{Id: ch.Id, Name: "c-swk-2", Status: 2, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 0}
	require.NoError(t, stale.SaveWithoutKey())

	var got Channel
	require.NoError(t, DB.First(&got, ch.Id).Error)
	assert.Equal(t, int64(70), got.UsedQuota, "SaveWithoutKey 不应覆盖 used_quota")
	assert.Equal(t, "c-swk-2", got.Name, "常规字段应正常更新")
}

// Channel.Save（GetSetting/GetOtherSettings 自愈路径）整行 Save 不得覆盖原子 used_quota。
func TestChannelSave_DoesNotClobberUsedQuota(t *testing.T) {
	truncateTables(t)

	ch := &Channel{Name: "c-save", Status: 1, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 3.0}
	require.NoError(t, DB.Create(ch).Error)

	updateChannelUsedQuota(ch.Id, 30)

	stale := &Channel{Id: ch.Id, Name: "c-save-2", Status: 1, Group: "default", Models: "gpt-4o", UsedQuota: 0, Balance: 0}
	require.NoError(t, stale.Save())

	var got Channel
	require.NoError(t, DB.First(&got, ch.Id).Error)
	assert.Equal(t, int64(30), got.UsedQuota, "Save 不应覆盖 used_quota")
	assert.Equal(t, "c-save-2", got.Name, "常规字段应正常更新")
}

// User.Edit 走 map 白名单，quota 不在其中——回归守卫，确保未来不会把 quota 误加入写集。
func TestUserEdit_DoesNotClobberQuota(t *testing.T) {
	truncateTables(t)

	u := &User{Username: "edit_lu", Password: "hashed", DisplayName: "old", Quota: 100, Group: "default"}
	require.NoError(t, DB.Create(u).Error)

	require.NoError(t, decreaseUserQuota(u.Id, 40)) // DB quota -> 60

	stale := &User{Id: u.Id, Username: "edit_lu", DisplayName: "newname", Quota: 100, Group: "default"}
	require.NoError(t, stale.Edit(false))

	var got User
	require.NoError(t, DB.First(&got, u.Id).Error)
	assert.Equal(t, 60, got.Quota, "Edit 不应覆盖 quota")
	assert.Equal(t, "newname", got.DisplayName, "常规字段应正常更新")
}
