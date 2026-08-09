package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Token struct {
	Id                 int     `json:"id"`
	UserId             int     `json:"user_id" gorm:"index"`
	Key                string  `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status             int     `json:"status" gorm:"default:1"`
	Name               string  `json:"name" gorm:"index" `
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	AccessedTime       int64   `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64   `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota        int     `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota     bool    `json:"unlimited_quota"`
	ModelLimitsEnabled bool    `json:"model_limits_enabled"`
	ModelLimits        string  `json:"model_limits" gorm:"type:text"`
	AllowIps           *string `json:"allow_ips" gorm:"default:''"`
	UsedQuota          int     `json:"used_quota" gorm:"default:0"` // used quota
	Group              string  `json:"group" gorm:"default:''"`
	CrossGroupRetry    bool    `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	PeriodType         string  `json:"period_type" gorm:"type:varchar(16)"`
	PeriodDays         int     `json:"period_days"`
	PeriodQuotaLimit   int64   `json:"period_quota_limit"`
	PeriodLimitUnit    string  `json:"period_limit_unit" gorm:"type:varchar(8)"`
	PeriodAnchorAt     int64   `json:"period_anchor_at"`
	PeriodStartAt      int64   `json:"period_start_at"`
	PeriodUsedQuota    int64   `json:"period_used_quota"`
	// These fields are read-only response projections. They are deliberately
	// excluded from the schema and are never accepted by token write DTOs.
	PeriodResetAt        int64          `json:"period_reset_at" gorm:"-"`
	PeriodRemainingQuota int64          `json:"period_remaining_quota" gorm:"-"`
	DeletedAt            gorm.DeletedAt `gorm:"index"`
}

// PeriodLimitEnabled reports whether the token has an active period policy.
// The policy is enabled solely by these two persisted values; no extra flag is
// used so legacy zero-valued rows remain disabled.
func (token *Token) PeriodLimitEnabled() bool {
	return token != nil && token.PeriodType != "" && token.PeriodQuotaLimit > 0
}

// TokenPeriodState is the database-only snapshot used by quota accounting and
// period gates. It intentionally has no Redis fallback.
type TokenPeriodState struct {
	Type      string `gorm:"column:period_type"`
	Days      int    `gorm:"column:period_days"`
	Limit     int64  `gorm:"column:period_quota_limit"`
	Unit      string `gorm:"column:period_limit_unit"`
	AnchorAt  int64  `gorm:"column:period_anchor_at"`
	StartAt   int64  `gorm:"column:period_start_at"`
	UsedQuota int64  `gorm:"column:period_used_quota"`
}

// TokenPeriodAdjustmentHint carries only a previously validated disabled
// decision. Enabled policies still reload authoritative counters for every
// adjustment.
type TokenPeriodAdjustmentHint struct {
	KnownDisabled bool
}

// PeriodLimitEnabled mirrors Token.PeriodLimitEnabled for a loaded snapshot.
func (state *TokenPeriodState) PeriodLimitEnabled() bool {
	return state != nil && state.Type != "" && state.Limit > 0
}

// CurrentStart returns the bucket start containing now, or zero for an invalid
// policy. Calendar arithmetic is kept in common so every caller uses UTC+8.
func (state TokenPeriodState) CurrentStart(now time.Time) int64 {
	start, _, ok := common.TokenPeriodBounds(state.Type, state.Days, state.AnchorAt, now)
	if !ok {
		return 0
	}
	return start
}

// ResetAt returns the exclusive end of the current bucket, or zero when the
// policy is invalid.
func (state TokenPeriodState) ResetAt(now time.Time) int64 {
	_, end, ok := common.TokenPeriodBounds(state.Type, state.Days, state.AnchorAt, now)
	if !ok {
		return 0
	}
	return end
}

// EffectiveUsed treats a persisted count from an older bucket as zero. The
// actual reset is performed atomically by the next quota adjustment.
func (state TokenPeriodState) EffectiveUsed(now time.Time) int64 {
	if !state.PeriodLimitEnabled() || state.StartAt != state.CurrentStart(now) || state.UsedQuota <= 0 {
		return 0
	}
	return state.UsedQuota
}

// LoadTokenPeriodState reads period configuration and counters directly from
// the primary database. Redis is intentionally not consulted.
func LoadTokenPeriodState(id int) (*TokenPeriodState, error) {
	if id <= 0 {
		return nil, errors.New("token id 无效")
	}
	if DB == nil {
		return nil, errors.New("主数据库未初始化")
	}
	state := &TokenPeriodState{}
	err := DB.Model(&Token{}).
		Select("COALESCE(period_type, '') AS period_type, COALESCE(period_days, 0) AS period_days, COALESCE(period_quota_limit, 0) AS period_quota_limit, COALESCE(period_limit_unit, '') AS period_limit_unit, COALESCE(period_anchor_at, 0) AS period_anchor_at, COALESCE(period_start_at, 0) AS period_start_at, COALESCE(period_used_quota, 0) AS period_used_quota").
		Where("id = ?", id).
		First(state).Error
	return state, err
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

func validateLikePattern(input string) error {
	// 1. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 2. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 3. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
	}

	return nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	tokens = make([]*Token, 0)
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := DB.Model(&Token{}).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Session(&gorm.Session{}).Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenInvalid
	}
	return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Where(commonKeyCol+" = ?", key).First(&token).Error
	return token, err
}

func (token *Token) Insert() error {
	var err error
	err = DB.Create(token).Error
	return err
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				// 失效而非整 hash 覆盖 RemainQuota（Lost Update）；失效后下次读从库重建。
				if err := cacheDeleteToken(token.Key); err != nil {
					common.SysLog("failed to invalidate token cache: " + err.Error())
				}
			})
		}
	}()
	// PeriodStartAt and PeriodUsedQuota are atomic accounting state. They must
	// never be part of this snapshot-based update (see token period #128).
	// The five policy fields are safe metadata and are included here; callers
	// that must update status alone use SelectUpdate instead.
	err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry",
		"period_type", "period_days", "period_quota_limit", "period_limit_unit", "period_anchor_at").Updates(token).Error
	return err
}

// UpdatePeriodConfig persists the controller-writable token fields together
// with a deliberate period reset/policy change. The atomic counters are kept
// out of Update's snapshot whitelist, but a policy transition still needs one
// explicit write so config and reset state cannot be split across requests.
func (token *Token) UpdatePeriodConfig() error {
	return token.updatePeriodConfig(true)
}

// UpdatePeriodConfigPreserveState changes policy metadata without copying the
// snapshot counters. It is used for limit/unit-only edits so an in-flight
// accounting SQL cannot be lost to a stale controller object.
func (token *Token) UpdatePeriodConfigPreserveState() error {
	return token.updatePeriodConfig(false)
}

func (token *Token) updatePeriodConfig(resetState bool) error {
	updates := map[string]interface{}{
		"name":                 token.Name,
		"status":               token.Status,
		"expired_time":         token.ExpiredTime,
		"remain_quota":         token.RemainQuota,
		"unlimited_quota":      token.UnlimitedQuota,
		"model_limits_enabled": token.ModelLimitsEnabled,
		"model_limits":         token.ModelLimits,
		"allow_ips":            token.AllowIps,
		"group":                token.Group,
		"cross_group_retry":    token.CrossGroupRetry,
		"period_type":          token.PeriodType,
		"period_days":          token.PeriodDays,
		"period_quota_limit":   token.PeriodQuotaLimit,
		"period_limit_unit":    token.PeriodLimitUnit,
		"period_anchor_at":     token.PeriodAnchorAt,
	}
	if resetState {
		updates["period_start_at"] = token.PeriodStartAt
		updates["period_used_quota"] = token.PeriodUsedQuota
	}
	result := DB.Model(&Token{}).Where("id = ?", token.Id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if common.RedisEnabled {
		// Period policy changes invalidate the entire projection. Never write the
		// freshly loaded struct back over accounting state in the Redis hash.
		if cacheErr := cacheDeleteToken(token.Key); cacheErr != nil {
			common.SysLog("failed to invalidate token cache: " + cacheErr.Error())
		}
	}
	return nil
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				// 失效而非整 hash 覆盖 RemainQuota（Lost Update）；失效后下次读从库重建。
				if err := cacheDeleteToken(token.Key); err != nil {
					common.SysLog("failed to invalidate token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return AdjustTokenQuota(tokenId, key, -quota, 0, nil)
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return AdjustTokenQuota(id, key, quota, 0, nil)
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// AdjustTokenQuota applies a signed quota delta. Positive values consume
// quota; negative values refund it. attributedPeriodStart identifies the
// original bucket for refunds and is zero for the current bucket.
func AdjustTokenQuota(id int, key string, delta int, attributedPeriodStart int64, hint *TokenPeriodAdjustmentHint) error {
	// Batch enqueue has no immediate RowsAffected check. Reload the row before
	// deciding whether it is eligible for the legacy queue so a stale disabled
	// decision cannot hide an already deleted or newly enabled token.
	if common.BatchUpdateEnabled {
		hint = nil
	}
	if err := adjustTokenQuota(id, delta, attributedPeriodStart, hint); err != nil {
		return err
	}
	if common.RedisEnabled {
		cacheDelta := -int64(delta)
		gopool.Go(func() {
			if err := cacheIncrTokenQuota(key, cacheDelta); err != nil {
				common.SysLog("failed to adjust token quota cache: " + err.Error())
			}
		})
	}
	return nil
}

// adjustTokenQuota is the single model-level accounting implementation. It
// selects the legacy batch path only after confirming that the token has no
// active period policy.
func adjustTokenQuota(id int, delta int, attributedPeriodStart int64, hint *TokenPeriodAdjustmentHint) error {
	var state *TokenPeriodState
	if hint == nil || !hint.KnownDisabled {
		var err error
		state, err = LoadTokenPeriodState(id)
		if err != nil {
			return err
		}
	}
	if state == nil || !state.PeriodLimitEnabled() {
		if common.BatchUpdateEnabled {
			addNewRecord(BatchUpdateTypeTokenQuota, id, -delta)
			return nil
		}
		return adjustTokenQuotaLegacy(id, delta)
	}

	now := time.Now()
	currentStart := state.CurrentStart(now)
	if currentStart <= 0 {
		return errors.New("令牌周期配置无效")
	}
	// A refund tied to an older bucket must not subtract from the new bucket.
	// Positive adjustments (for example a late task settlement) still need to
	// be counted in the current bucket because there is no historical bucket
	// table to mutate.
	if delta < 0 && attributedPeriodStart > 0 && attributedPeriodStart != currentStart {
		return adjustTokenQuotaAttributedPeriod(id, delta)
	}

	result := DB.Exec(`UPDATE tokens
SET period_used_quota =
      CASE
        WHEN COALESCE(period_start_at,0) = ? THEN
             CASE WHEN COALESCE(period_used_quota,0) + ? < 0 THEN 0
                  ELSE COALESCE(period_used_quota,0) + ? END
        WHEN ? > 0 THEN ?
        ELSE 0
      END,
    period_start_at = ?,
    remain_quota = remain_quota - ?,
    used_quota = used_quota + ?,
    accessed_time = ?
WHERE id = ? AND deleted_at IS NULL`,
		currentStart,
		int64(delta),
		int64(delta),
		int64(delta),
		int64(delta),
		currentStart,
		int64(delta),
		int64(delta),
		common.GetTimestamp(),
		id,
	)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// adjustTokenQuotaLegacy keeps the pre-period behavior byte-for-byte in terms
// of arithmetic and batch semantics for tokens without a period policy.
func adjustTokenQuotaLegacy(id int, delta int) error {
	result := DB.Model(&Token{}).Where("id = ?", id).Updates(map[string]interface{}{
		"remain_quota":  gorm.Expr("remain_quota - ?", delta),
		"used_quota":    gorm.Expr("used_quota + ?", delta),
		"accessed_time": common.GetTimestamp(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// A refund attributed to an older bucket must not reset or mutate the current
// period counters. The balance columns remain one atomic update with it.
func adjustTokenQuotaAttributedPeriod(id int, delta int) error {
	result := DB.Exec(`UPDATE tokens
SET remain_quota = remain_quota - ?,
    used_quota = used_quota + ?,
    accessed_time = ?
WHERE id = ? AND deleted_at IS NULL`,
		int64(delta), int64(delta), common.GetTimestamp(), id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := cacheDeleteToken(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
