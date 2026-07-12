package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

func GetUserRecentSpend(userId int, hours int) (float64, error) {
	since := common.GetTimestamp() - int64(hours*3600)
	var total int64
	err := LOG_DB.Table("logs").
		Where("user_id = ? AND type = ? AND created_at >= ?", userId, LogTypeConsume, since).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return float64(total) / common.QuotaPerUnit, nil
}

func GetUserYesterdaySpend(userId int) (float64, error) {
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location()).Unix()
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	var total int64
	err := LOG_DB.Table("logs").
		Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?", userId, LogTypeConsume, yesterdayStart, yesterdayEnd).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return float64(total) / common.QuotaPerUnit, nil
}

func GetUserRecentTopupQuota(userId int, hours int) (int64, error) {
	since := common.GetTimestamp() - int64(hours*3600)
	var total int64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND (source IS NULL OR source = '' OR source != ?) AND create_time >= ?",
			userId, common.TopUpStatusSuccess, "discount_bonus", since).
		Select("COALESCE(SUM(quota_granted), 0)").
		Scan(&total).Error
	return total, err
}

func GetUserYesterdayTopupQuota(userId int) (int64, error) {
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location()).Unix()
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	var total int64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND (source IS NULL OR source = '' OR source != ?) AND create_time >= ? AND create_time < ?",
			userId, common.TopUpStatusSuccess, "discount_bonus", yesterdayStart, yesterdayEnd).
		Select("COALESCE(SUM(quota_granted), 0)").
		Scan(&total).Error
	return total, err
}

func GetUserRecentRequestCount(userId int, hours int) (int64, error) {
	since := common.GetTimestamp() - int64(hours*3600)
	var count int64
	err := LOG_DB.Table("logs").
		Where("user_id = ? AND type = ? AND created_at >= ?", userId, LogTypeConsume, since).
		Count(&count).Error
	return count, err
}

func GetUserNetTopupQuota(userId int) (int64, error) {
	payment, err := GetUserPaymentTopUpQuota(userId)
	if err != nil {
		return 0, err
	}
	refunded, err := GetUserTotalRefundedQuota(userId)
	if err != nil {
		return 0, err
	}
	net := payment - refunded
	if net < 0 {
		net = 0
	}
	return net, nil
}
