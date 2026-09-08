package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

var (
	ErrUserQuotaInsufficient = errors.New("user quota insufficient")
	ErrUserNotFound          = errors.New("user not found")
	ErrInvalidQuotaReserve   = errors.New("quota reserve amount must be positive")
)

// ReserveUserQuota conditionally deducts a wallet amount. A false result
// means the user exists but does not have enough quota.
func ReserveUserQuota(userId int, amount int) (reserved bool, err error) {
	if amount <= 0 {
		return false, ErrInvalidQuotaReserve
	}

	if common.BatchUpdateEnabled {
		// Compatibility fallback: batch updates retain today's unconditional
		// deduction semantics, so enabling the performance switch does not turn
		// the reserve path into a site-wide rejection gate.
		err := DecreaseUserQuota(userId, amount, false)
		return true, err
	}

	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", userId, amount).
		Update("quota", gorm.Expr("quota - ?", amount))
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		gopool.Go(func() {
			if cacheErr := cacheDecrUserQuota(userId, int64(amount)); cacheErr != nil {
				if errors.Is(cacheErr, common.ErrRedisKeyMiss) {
					if invalidateErr := invalidateUserCache(userId); invalidateErr != nil {
						common.SysError("failed to invalidate user cache after reserve cache miss: " + invalidateErr.Error())
					}
					return
				}
				common.SysLog("failed to decrease user quota cache after reserve: " + cacheErr.Error())
			}
		})
		return true, nil
	}

	var user User
	err = DB.Select("id").First(&user, userId).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrUserNotFound
		}
		return false, err
	}
	return false, nil
}
