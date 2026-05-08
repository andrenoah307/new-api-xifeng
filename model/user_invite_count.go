package model

import (
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"

	"github.com/samber/hot"
)

const (
	inviteCountCacheNamespace = "new-api:invite_count:v1"
	inviteCountCacheTTL       = 5 * time.Minute
	inviteCountCacheCapacity  = 1000
)

var (
	inviteCountCache     *cachex.HybridCache[int]
	inviteCountCacheOnce sync.Once
)

func getInviteCountCache() *cachex.HybridCache[int] {
	inviteCountCacheOnce.Do(func() {
		inviteCountCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(inviteCountCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, inviteCountCacheCapacity).
					WithTTL(inviteCountCacheTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return inviteCountCache
}

func GetInviteCount(userId int) int {
	if userId <= 0 {
		return 0
	}
	key := strconv.Itoa(userId)
	cache := getInviteCountCache()
	if v, found, _ := cache.Get(key); found {
		return v
	}
	var count int64
	DB.Model(&User{}).Where("inviter_id = ?", userId).Count(&count)
	_ = cache.SetWithTTL(key, int(count), inviteCountCacheTTL)
	return int(count)
}

func InvalidateInviteCount(inviterId int) {
	if inviterId <= 0 {
		return
	}
	cache := getInviteCountCache()
	_, _ = cache.DeleteMany([]string{strconv.Itoa(inviterId)})
}
