package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// TestShouldSendQuotaWarningEdge_Memory 验证额度预警的边沿触发 + 滞回（内存回退路径）。
func TestShouldSendQuotaWarningEdge_Memory(t *testing.T) {
	origRedis := common.RedisEnabled
	common.RedisEnabled = false
	defer func() { common.RedisEnabled = origRedis }()

	key := "quota_warn_state:test-user-1"
	quotaWarnStateStore.Delete(key)

	// 首次跌破阈值 -> 发送一次
	if !shouldSendQuotaWarningEdge(key, true) {
		t.Fatal("首次跌破阈值应当通知")
	}
	// 持续低于阈值 -> 不再重复
	if shouldSendQuotaWarningEdge(key, true) {
		t.Fatal("持续低于阈值不应重复通知")
	}
	if shouldSendQuotaWarningEdge(key, true) {
		t.Fatal("持续低于阈值不应重复通知(2)")
	}
	// 回升到阈值之上 -> 不发送，并清除状态
	if shouldSendQuotaWarningEdge(key, false) {
		t.Fatal("余额回升不应通知")
	}
	// 再次跌破 -> 再次发送
	if !shouldSendQuotaWarningEdge(key, true) {
		t.Fatal("回升后再次跌破应再次通知")
	}
	// 再次持续低于 -> 不重复
	if shouldSendQuotaWarningEdge(key, true) {
		t.Fatal("再次持续低于阈值不应重复通知")
	}
}
