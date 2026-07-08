package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func seedTopUpForExclude(t *testing.T, userId int, tradeNo, source string) {
	t.Helper()
	tu := &TopUp{
		UserId:       userId,
		Money:        10,
		QuotaGranted: 5000,
		TradeNo:      tradeNo,
		Status:       "success",
		Source:       source,
		CreateTime:   common.GetTimestamp(),
		CompleteTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(tu).Error)
}

func TestGetUserTopUpsExcludeSourcesFiltersDiscountBonus(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM top_ups").Error)
	const uid = 90001
	seedTopUpForExclude(t, uid, "ex-normal-1", "")
	seedTopUpForExclude(t, uid, "ex-normal-2", "online")
	seedTopUpForExclude(t, uid, "ex-normal-3", "")
	seedTopUpForExclude(t, uid, "ex-bonus-1", "discount_bonus")
	seedTopUpForExclude(t, uid, "ex-bonus-2", "discount_bonus")

	filter := TopUpFilter{ExcludeSources: []string{"discount_bonus"}}
	pageInfo := &common.PageInfo{Page: 1, PageSize: 5}
	items, total, err := GetUserTopUps(uid, filter, pageInfo)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	for _, it := range items {
		require.NotEqual(t, "discount_bonus", it.Source)
	}
}

func TestGetUserTopUpsExcludeSourcesPagination(t *testing.T) {
	require.NoError(t, DB.Exec("DELETE FROM top_ups").Error)
	const uid = 90002
	for i := 0; i < 7; i++ {
		seedTopUpForExclude(t, uid, fmt.Sprintf("pg-%d", i), "")
	}

	filter := TopUpFilter{ExcludeSources: []string{"discount_bonus"}}
	page1 := &common.PageInfo{Page: 1, PageSize: 5}
	items1, total1, err := GetUserTopUps(uid, filter, page1)
	require.NoError(t, err)
	require.Equal(t, int64(7), total1)
	require.Len(t, items1, 5)

	page2 := &common.PageInfo{Page: 2, PageSize: 5}
	items2, _, err := GetUserTopUps(uid, filter, page2)
	require.NoError(t, err)
	require.Len(t, items2, 2)
}
