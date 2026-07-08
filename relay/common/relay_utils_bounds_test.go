package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateTaskDurationBounds 守住计费不变式：用户传入的视频时长（seconds/duration）
// 作为计费乘子（OtherRatio "seconds"），超过 MaxTaskDurationSeconds 或为负时必须被请求
// 校验拒绝，避免异常值溢出额度转换（恶性余额负数溢出）。
func TestValidateTaskDurationBounds(t *testing.T) {
	assert.Equal(t, 3600, MaxTaskDurationSeconds)

	// 合法：0（未指定，交由下游默认）、正常时长、恰好等于上限。
	assert.Nil(t, validateTaskDurationBounds(TaskSubmitReq{Duration: 0}))
	assert.Nil(t, validateTaskDurationBounds(TaskSubmitReq{Duration: 8}))
	assert.Nil(t, validateTaskDurationBounds(TaskSubmitReq{Duration: MaxTaskDurationSeconds}))

	// 超限：Duration 与字符串 Seconds 两条来源都要拦。
	assert.NotNil(t, validateTaskDurationBounds(TaskSubmitReq{Duration: MaxTaskDurationSeconds + 1}))
	assert.NotNil(t, validateTaskDurationBounds(TaskSubmitReq{Seconds: "999999999"}))
	// 负值（含回绕）拒绝。
	assert.NotNil(t, validateTaskDurationBounds(TaskSubmitReq{Duration: -1}))
}
