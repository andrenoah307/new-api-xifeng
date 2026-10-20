package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

var fundingOverviewCacheOnce sync.Once
var fundingOverviewCache *cachex.HybridCache[model.FundingOverview]
var fundingOverviewFlight singleflight.Group

func GetFundingOverview(ctx context.Context, start, end int64) (model.FundingOverview, error) {
	if !model.ValidFundingOverviewRange(start, end, time.Now().Unix()) || !common.DataExportEnabled {
		return model.FundingOverview{}, model.ErrFundingOverviewUnavailable
	}
	fundingOverviewCacheOnce.Do(func() {
		fundingOverviewCache = cachex.NewHybridCache(cachex.HybridCacheConfig[model.FundingOverview]{
			Namespace: "funding_overview:v1", Redis: common.RDB,
			RedisEnabled: func() bool { return common.RedisEnabled },
			RedisCodec:   cachex.JSONCodec[model.FundingOverview]{},
			Memory: func() *hot.HotCache[string, model.FundingOverview] {
				return hot.NewHotCache[string, model.FundingOverview](hot.LRU, 64).Build()
			},
		})
	})
	key := fmt.Sprintf("%d:%d:%g:%g", start, end, common.QuotaPerUnit, operation_setting.USDExchangeRate)
	value, found, err := fundingOverviewCache.Get(key)
	if err != nil {
		return value, err
	} // Redis failure must not fan out into DB scans.
	if found {
		return value, nil
	}
	ch := fundingOverviewFlight.DoChan(key, func() (interface{}, error) {
		cached, ok, err := fundingOverviewCache.Get(key)
		if err != nil || ok {
			return cached, err
		}
		value, err := model.QueryFundingOverview(ctx, start, end)
		if err != nil {
			return value, err
		}
		if err := fundingOverviewCache.SetWithTTL(key, value, 5*time.Minute); err != nil {
			return value, err
		}
		return value, nil
	})
	select {
	case <-ctx.Done():
		return model.FundingOverview{}, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return model.FundingOverview{}, result.Err
		}
		return result.Val.(model.FundingOverview), nil
	}
}
