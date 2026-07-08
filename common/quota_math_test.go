package common

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// 2000 quota per call * n=1.8446744073686647e19 溢出 int64，用于复现「价格 × 巨大
// 用户传入次数」的超范围乘积，验证饱和而非回绕成负数返利。
const overflowingProduct = 2000 * 1.8446744073686647e19

// TestQuotaFromFloat 守住计费不变式：超范围额度乘积饱和到 int32 上限，绝不回绕成
// 负数（返利）。QuotaFromFloat 向零截断。
func TestQuotaFromFloat(t *testing.T) {
	assert.Equal(t, 42, QuotaFromFloat(42.4))
	assert.Equal(t, 42, QuotaFromFloat(42.9))
	assert.Equal(t, -42, QuotaFromFloat(-42.9))
	assert.Equal(t, MaxQuota, QuotaFromFloat(overflowingProduct))
	assert.Equal(t, MinQuota, QuotaFromFloat(-overflowingProduct))
	assert.Equal(t, MaxQuota, QuotaFromFloat(math.Inf(1)))
	assert.Equal(t, MinQuota, QuotaFromFloat(math.Inf(-1)))
	assert.Equal(t, 0, QuotaFromFloat(math.NaN()))
}

// TestQuotaRound 验证「四舍五入远离零」+ 同样的饱和策略。
func TestQuotaRound(t *testing.T) {
	assert.Equal(t, 42, QuotaRound(41.5))
	assert.Equal(t, 43, QuotaRound(42.5))
	assert.Equal(t, -43, QuotaRound(-42.5))
	assert.Equal(t, MaxQuota, QuotaRound(overflowingProduct))
	assert.Equal(t, MinQuota, QuotaRound(-overflowingProduct))
	assert.Equal(t, 0, QuotaRound(math.NaN()))
}

// TestQuotaFromDecimal 验证 decimal 先四舍五入再饱和转换。
func TestQuotaFromDecimal(t *testing.T) {
	assert.Equal(t, 43, QuotaFromDecimal(decimal.NewFromFloat(42.5)))
	assert.Equal(t, -43, QuotaFromDecimal(decimal.NewFromFloat(-42.5)))
	// decimal.New(1, 20) = 1e20，远超 int32 上限，应饱和到 MaxQuota。
	assert.Equal(t, MaxQuota, QuotaFromDecimal(decimal.New(1, 20)))
	assert.Equal(t, MinQuota, QuotaFromDecimal(decimal.New(-1, 20)))
}
