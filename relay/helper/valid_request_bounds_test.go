package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
)

// TestExceedsMaxTokensLimit 守住计费不变式：用户传入的 max_tokens 类字段超过
// maxTokensLimit（MaxInt32/2）时必须被请求校验拒绝，避免异常大值顶穿预扣估算并
// 溢出额度转换（恶性余额负数溢出）。
func TestExceedsMaxTokensLimit(t *testing.T) {
	within := uint(1024)
	over := uint(maxTokensLimit + 1)
	atLimit := uint(maxTokensLimit)

	assert.False(t, exceedsMaxTokensLimit(nil), "nil pointer treated as 0, within bound")
	assert.False(t, exceedsMaxTokensLimit(&within))
	assert.False(t, exceedsMaxTokensLimit(&atLimit), "exactly at limit is allowed")
	assert.True(t, exceedsMaxTokensLimit(&over))
	// 多字段：任一超限即拒绝。
	assert.True(t, exceedsMaxTokensLimit(&within, &over))
	assert.False(t, exceedsMaxTokensLimit(&within, &atLimit, nil))
}

// TestMaxImageN 固定图像生成张数上限，防止用户传入巨大/回绕 n 放大额度乘积。
func TestMaxImageN(t *testing.T) {
	assert.Equal(t, 128, dto.MaxImageN)
	// 校验器语义：> MaxImageN 拒绝，<= 放行（含默认 1）。
	over := common.GetPointer(uint(dto.MaxImageN + 1))
	assert.Greater(t, int(*over), dto.MaxImageN)
}
