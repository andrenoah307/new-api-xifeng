package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	QuotaGranted    int64   `json:"quota_granted" gorm:"default:0"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
	Source          string  `json:"source" gorm:"type:varchar(50);default:''"`
	DiscountCodeId  int     `json:"discount_code_id" gorm:"default:0"`
}

type TopUpWithUsername struct {
	TopUp
	Username *string `json:"username,omitempty" gorm:"column:username"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		quota = topUp.Money * common.QuotaPerUnit
		topUp.QuotaGranted = int64(quota)
		err = tx.Save(topUp).Error
		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(int(quota)), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)
	GrantTopUpCommission(topUp, false)
	ProcessDiscountCodeBonus(topUp)

	return nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// TopUpFilter holds optional filter conditions for topup queries.
type TopUpFilter struct {
	Keyword   string
	Status    string
	StartTime int64
	EndTime   int64
}

// GetUserTopUps 查询某用户的充值记录（分页 + COUNT 上限防 DoS）。
func GetUserTopUps(userId int, filter TopUpFilter, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	topups = make([]*TopUp, 0)
	query := tx.Model(&TopUp{}).Where("user_id = ?", userId)
	query, err = applyTopUpQueryFilters(query, filter)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Session(&gorm.Session{}).Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count user topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to query user topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(filter TopUpFilter, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	topups = make([]*TopUp, 0)
	query := tx.Model(&TopUp{})
	query, err = applyTopUpQueryFilters(query, filter)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = query.Session(&gorm.Session{}).Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to query topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// GetAllTopUpsWithUsername 管理员获取充值记录，LEFT JOIN users 取 username
func GetAllTopUpsWithUsername(filter TopUpFilter, pageInfo *common.PageInfo) (topups []*TopUpWithUsername, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	topups = make([]*TopUpWithUsername, 0)
	needJoin := filter.Keyword != "" && !isNumericString(filter.Keyword)

	countQuery := tx.Model(&TopUp{})
	if needJoin {
		countQuery = countQuery.Joins("LEFT JOIN users ON users.id = top_ups.user_id")
	}
	countQuery, err = applyTopUpAdminFilters(countQuery, filter)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = countQuery.Session(&gorm.Session{}).Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	dataQuery := tx.Model(&TopUp{}).
		Select("top_ups.*, users.username AS username").
		Joins("LEFT JOIN users ON users.id = top_ups.user_id")
	dataQuery, err = applyTopUpAdminFilters(dataQuery, filter)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = dataQuery.Order("top_ups.id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to query topups: " + err.Error())
		return nil, 0, errors.New("查询充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

func isNumericString(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// applyTopUpAdminFilters 管理员查询：keyword 搜索 trade_no / user_id / username
func applyTopUpAdminFilters(query *gorm.DB, filter TopUpFilter) (*gorm.DB, error) {
	if filter.Keyword != "" {
		pattern, err := sanitizeLikePattern(filter.Keyword)
		if err != nil {
			return query, err
		}
		if !strings.Contains(pattern, "%") {
			pattern = "%" + pattern + "%"
		}
		if uid, parseErr := strconv.Atoi(filter.Keyword); parseErr == nil {
			query = query.Where("(top_ups.trade_no LIKE ? ESCAPE '!' OR top_ups.user_id = ?)", pattern, uid)
		} else {
			query = query.Where("(top_ups.trade_no LIKE ? ESCAPE '!' OR users.username LIKE ? ESCAPE '!')", pattern, pattern)
		}
	}
	if filter.Status != "" {
		query = query.Where("top_ups.status = ?", filter.Status)
	}
	if filter.StartTime > 0 {
		query = query.Where("top_ups.create_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("top_ups.create_time <= ?", filter.EndTime)
	}
	return query, nil
}

// applyTopUpQueryFilters applies keyword / status / time range filters to a GORM query.
func applyTopUpQueryFilters(query *gorm.DB, filter TopUpFilter) (*gorm.DB, error) {
	if filter.Keyword != "" {
		pattern, err := sanitizeLikePattern(filter.Keyword)
		if err != nil {
			return query, err
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.StartTime > 0 {
		query = query.Where("create_time >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("create_time <= ?", filter.EndTime)
	}
	return query, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var topUpForCommission *TopUp

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).IntPart())
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		topUp.QuotaGranted = int64(quotaToAdd)
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		topUpForCommission = topUp
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	if topUpForCommission != nil {
		GrantTopUpCommission(topUpForCommission, true)
	}
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount
		topUp.QuotaGranted = quota
		err = tx.Save(topUp).Error

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)
	GrantTopUpCommission(topUp, false)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		topUp.QuotaGranted = int64(quotaToAdd)
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
		GrantTopUpCommission(topUp, false)
		ProcessDiscountCodeBonus(topUp)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		topUp.QuotaGranted = int64(quotaToAdd)
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
		GrantTopUpCommission(topUp, false)
		ProcessDiscountCodeBonus(topUp)
	}

	return nil
}

func GetUserTotalTopUpQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(quota_granted), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

func GetUserPaymentTopUpQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND (source IS NULL OR source = '' OR source != ?)",
			userId, common.TopUpStatusSuccess, "discount_bonus").
		Select("COALESCE(SUM(quota_granted), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

func GetUserBonusTopUpQuota(userId int) (int64, error) {
	var total int64
	err := DB.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND source = ?",
			userId, common.TopUpStatusSuccess, "discount_bonus").
		Select("COALESCE(SUM(quota_granted), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

// ProcessDiscountCodeBonus creates a bonus TopUp record for discount code orders.
// Called after a successful payment recharge. Idempotent: skips if bonus already exists.
func ProcessDiscountCodeBonus(topUp *TopUp) {
	if topUp.DiscountCodeId <= 0 || topUp.QuotaGranted <= 0 {
		return
	}
	dc, err := GetDiscountCodeById(topUp.DiscountCodeId)
	if err != nil || dc == nil || dc.DiscountRate <= 0 || dc.DiscountRate >= 100 {
		return
	}

	bonusTradeNo := topUp.TradeNo + "_bonus"
	existing := GetTopUpByTradeNo(bonusTradeNo)
	if existing != nil {
		return
	}

	dPaid := decimal.NewFromInt(topUp.QuotaGranted)
	dRate := decimal.NewFromInt(int64(dc.DiscountRate))
	dHundred := decimal.NewFromInt(100)
	originalQuota := dPaid.Mul(dHundred).Div(dRate).IntPart()
	bonusQuota := originalQuota - topUp.QuotaGranted
	if bonusQuota <= 0 {
		return
	}

	bonusTopUp := &TopUp{
		UserId:          topUp.UserId,
		Amount:          0,
		Money:           0,
		QuotaGranted:    bonusQuota,
		TradeNo:         bonusTradeNo,
		PaymentMethod:   "discount_bonus",
		PaymentProvider: "discount_code",
		Source:          "discount_bonus",
		DiscountCodeId:  topUp.DiscountCodeId,
		CreateTime:      common.GetTimestamp(),
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	if err := bonusTopUp.Insert(); err != nil {
		common.SysError("failed to insert discount bonus topup: " + err.Error())
		return
	}

	if err := IncreaseUserQuota(topUp.UserId, int(bonusQuota), false); err != nil {
		common.SysError("failed to increase user quota for discount bonus: " + err.Error())
		return
	}

	_ = RecordDiscountCodeUsage(topUp.DiscountCodeId, topUp.UserId, topUp.Id)
	_ = DB.Model(&DiscountCode{}).Where("id = ?", topUp.DiscountCodeId).
		Update("used_count", gorm.Expr("used_count + 1")).Error

	RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("折扣码赠金 %s，折扣码ID %d", logger.FormatQuota(int(bonusQuota)), topUp.DiscountCodeId))
}
