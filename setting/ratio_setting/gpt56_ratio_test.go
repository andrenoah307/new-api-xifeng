package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetHardcodedCompletionModelRatio_GPT5Unlock 锁定上游「prepare for 5.6」的补全倍率语义：
// 无小数点的 gpt-5 家族（gpt-5 / gpt-5-mini / gpt-5-nano / 带日期）硬编码锁定 8；gpt-5.4 家族锁 6
// （nano 6.25）；gpt-5.5 及更新（gpt-5.6-*）解锁（locked=false）→ 交由管理员配置的 CompletionRatio。
func TestGetHardcodedCompletionModelRatio_GPT5Unlock(t *testing.T) {
	tests := []struct {
		name       string
		wantRatio  float64
		wantLocked bool
	}{
		{"gpt-5", 8, true},
		{"gpt-5-mini", 8, true},
		{"gpt-5-nano", 8, true},
		{"gpt-5-2025-08-07", 8, true},
		{"gpt-5.4", 6, true},
		{"gpt-5.4-nano", 6.25, true},
		{"gpt-5.5", 6, false},
		{"gpt-5.6-sol", 6, false},
		{"gpt-5.6-terra", 6, false},
		{"gpt-5.6-luna", 6, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, locked := getHardcodedCompletionModelRatio(tt.name)
			assert.Equal(t, tt.wantLocked, locked, "locked flag for %s", tt.name)
			assert.Equal(t, tt.wantRatio, ratio, "ratio for %s", tt.name)
		})
	}
}

// TestDefaultModelRatio_GPT56Present 固定上游 6ce7305cd 补入的 gpt-5.5/5.6 默认输入倍率，
// 避免这些模型未配置时回退 37.5 兜底哨兵（预扣虚高/被拒，坑点 #137）。
func TestDefaultModelRatio_GPT56Present(t *testing.T) {
	assert.Equal(t, 2.5, defaultModelRatio["gpt-5.5"])
	assert.Equal(t, 2.5, defaultModelRatio["gpt-5.6-sol"])
	assert.Equal(t, 1.25, defaultModelRatio["gpt-5.6-terra"])
	assert.Equal(t, 0.5, defaultModelRatio["gpt-5.6-luna"])
}

// TestGetCompletionRatio_GPT56HonorsConfigAfterUnlock 验证解锁后的计费契约：
// gpt-5.6-* 采用管理员配置的 CompletionRatio（此前被硬编码 8 覆盖）；无配置则回退 6
// （与解锁前 gpt-5.5 行为一致，保护高频 gpt-5.5 流量零变化）。
func TestGetCompletionRatio_GPT56HonorsConfigAfterUnlock(t *testing.T) {
	orig := CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateCompletionRatioByJSONString(orig))
	})

	require.NoError(t, UpdateCompletionRatioByJSONString(`{"gpt-5.6-sol":6,"gpt-5.6-luna":0.1}`))

	// 配置生效（解锁前会被硬编码 8 覆盖）
	assert.Equal(t, float64(6), GetCompletionRatio("gpt-5.6-sol"))
	assert.Equal(t, 0.1, GetCompletionRatio("gpt-5.6-luna"))
	// 无配置 → 回退硬编码 6（gpt-5.5 高频流量不受影响）
	assert.Equal(t, float64(6), GetCompletionRatio("gpt-5.5"))
}
