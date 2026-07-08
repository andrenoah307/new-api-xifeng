package common

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// 额度转换集中在此，让所有计费路径共享同一套「饱和 + 记录」策略。
// 数据库中 user/token/log 的 quota 列是 32 位整数，因此超范围的乘积必须
// 钳制到 int32 区间，而不能回绕（wraparound）——回绕会把一次扣费变成返利
// （恶性余额负数溢出）。
const (
	MaxQuota = math.MaxInt32
	MinQuota = math.MinInt32
)

// saturateQuota 把一个已算好的额度浮点值转换为 int，并钳制到 int32 区间。
// 一旦发生钳制（否则就是整数回绕）或 NaN 兜底，都会记一条 SysError：正常一次
// 请求绝不会逼近这些边界，命中即意味着 bug 或滥用请求。op 标识调用方。
func saturateQuota(value float64, op string) int {
	switch {
	case math.IsNaN(value):
		SysError(fmt.Sprintf("quota conversion (%s) received NaN, falling back to 0", op))
		return 0
	case value >= MaxQuota:
		SysError(fmt.Sprintf("quota conversion (%s) overflow: %g exceeds max quota, clamped to %d", op, value, MaxQuota))
		return MaxQuota
	case value <= MinQuota:
		SysError(fmt.Sprintf("quota conversion (%s) underflow: %g below min quota, clamped to %d", op, value, MinQuota))
		return MinQuota
	default:
		return int(value)
	}
}

// QuotaFromFloat 把计算出的额度浮点值转换为 int（向零截断）并做饱和保护。
// 用于价格、倍率与用户可控乘子（图像 n、视频时长、分辨率倍率）的浮点乘积。
func QuotaFromFloat(value float64) int {
	return saturateQuota(value, "QuotaFromFloat")
}

// QuotaRound 把额度浮点值按「四舍五入远离零」转换为 int 并做饱和保护。
// 分层/表达式计费的各路径（预扣、结算、明细校验、日志字段）都应经此转换以避免 ±1 偏差。
func QuotaRound(value float64) int {
	return saturateQuota(math.Round(value), "QuotaRound")
}

// QuotaFromDecimal 把额度 decimal 转换为 int 并做饱和保护；转换前先四舍五入（远离零）。
func QuotaFromDecimal(d decimal.Decimal) int {
	f, _ := d.Round(0).Float64()
	return saturateQuota(f, "QuotaFromDecimal")
}
