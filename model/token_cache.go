package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func cacheSetToken(token Token) error {
	key := common.GenerateHMAC(token.Key)
	cachedToken := tokenCacheProjection(token)
	err := common.RedisHSetObj(fmt.Sprintf("token:%s", key), &cachedToken, time.Duration(common.RedisKeyCacheSeconds())*time.Second)
	if err != nil {
		return err
	}
	return nil
}

func tokenCacheProjection(token Token) Token {
	// RedisHSetObj reflects every struct field. Keep the accounting counters out
	// of the cache projection and do this on a copy so callers are untouched.
	cachedToken := token
	cachedToken.PeriodStartAt = -1
	cachedToken.PeriodUsedQuota = -1
	cachedToken.PeriodResetAt = 0
	cachedToken.PeriodRemainingQuota = 0
	cachedToken.Clean()
	return cachedToken
}

func cacheDeleteToken(key string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisDelKey(fmt.Sprintf("token:%s", key))
	if err != nil {
		return err
	}
	return nil
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	hmacKey := common.GenerateHMAC(key)
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(fmt.Sprintf("token:%s", hmacKey), &token)
	if err != nil {
		return nil, err
	}
	if token.PeriodStartAt != -1 || token.PeriodUsedQuota != -1 {
		// RedisHGetObj leaves fields missing from an old hash at their Go zero
		// values. Only the exact marker pair identifies the current projection;
		// missing, partial, or otherwise polluted markers must be rebuilt from DB.
		return nil, fmt.Errorf("token period state must be loaded from database")
	}
	token.Key = key
	// Cache contract: PeriodType, PeriodDays, PeriodQuotaLimit,
	// PeriodLimitUnit, and PeriodAnchorAt are usable policy metadata.
	// PeriodStartAt, PeriodUsedQuota, PeriodResetAt, and PeriodRemainingQuota
	// are unavailable; period consumers must call LoadTokenPeriodState.
	return &token, nil
}
