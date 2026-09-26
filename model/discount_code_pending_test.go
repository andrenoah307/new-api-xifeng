package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscountCodePendingLimitMessages(t *testing.T) {
	for _, scope := range []string{"user", "total"} {
		for _, status := range []string{"pending", "success", "old pending"} {
			t.Run(scope+"/"+status, func(t *testing.T) {
				setupDiscountCodeTest(t)
				t.Setenv("DISCOUNT_CODE_PENDING_TTL_SECONDS", "1800")
				dc := DiscountCode{Code: "pending", DiscountRate: 90}
				if scope == "user" {
					dc.MaxUsesPerUser = 1
				} else {
					dc.MaxUsesTotal = 1
				}
				require.NoError(t, dc.Insert())
				order := TopUp{TradeNo: "order", UserId: 1, DiscountCodeId: dc.Id, Status: status, CreateTime: common.GetTimestamp()}
				if status == "old pending" {
					order.Status = "pending"
					order.CreateTime -= 1801
				}
				require.NoError(t, DB.Create(&order).Error)
				// Bonus rows must not change a permanent-limit rejection into a pending hint.
				require.NoError(t, DB.Create(&TopUp{TradeNo: "bonus", UserId: 1, DiscountCodeId: dc.Id, Status: "pending", Source: "discount_bonus", CreateTime: common.GetTimestamp()}).Error)
				_, err := ValidateDiscountCode(dc.Code, 1)
				if status == "old pending" {
					require.NoError(t, err)
					return
				}
				want := "该折扣码使用次数已达上限"
				if scope == "user" {
					want = "您已达到该折扣码的使用次数上限"
				}
				if status == "pending" {
					want = "该折扣码名额暂被待支付订单占用，请稍后再试"
					if scope == "user" {
						want = "您有使用该折扣码的待支付订单，请先完成支付；未支付的订单超过 30 分钟后自动释放名额"
					}
				}
				require.EqualError(t, err, want)
			})
		}
	}
}

func TestDiscountCodePendingDuration(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		want    string
	}{{1800, "30 分钟"}, {3600, "1 小时"}, {90, "90 秒"}} {
		assert.Equal(t, tc.want, formatDiscountCodePendingDuration(tc.seconds))
	}
}
