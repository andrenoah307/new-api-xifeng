package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const tokenPeriodMaxDays = 3650

type normalizedTokenPeriod struct {
	periodType string
	periodDays int
	limit      int64
	unit       string
}

func tokenBadRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": err.Error(),
	})
}

func normalizeTokenPeriod(req dto.TokenRequest) (normalizedTokenPeriod, error) {
	periodType := strings.TrimSpace(req.PeriodType)
	unit := strings.TrimSpace(req.PeriodLimitUnit)
	value := strings.TrimSpace(req.PeriodLimitValue)

	switch periodType {
	case "":
		if unit != "" && unit != "cny" && unit != "quota" {
			return normalizedTokenPeriod{}, errors.New("period_limit_unit 无效")
		}
		if value != "" {
			parsed, err := decimal.NewFromString(value)
			if err != nil || !parsed.IsZero() {
				return normalizedTokenPeriod{}, errors.New("禁用周期限额时 period_limit_value 必须为 0")
			}
		}
		return normalizedTokenPeriod{}, nil
	case common.TokenPeriodTypeDays, common.TokenPeriodTypeWeek, common.TokenPeriodTypeMonth:
	default:
		return normalizedTokenPeriod{}, errors.New("period_type 无效")
	}

	if unit != "cny" && unit != "quota" {
		return normalizedTokenPeriod{}, errors.New("period_limit_unit 无效")
	}
	if value == "" {
		return normalizedTokenPeriod{}, errors.New("period_limit_value 不能为空")
	}
	if periodType == common.TokenPeriodTypeDays && (req.PeriodDays < 1 || req.PeriodDays > tokenPeriodMaxDays) {
		return normalizedTokenPeriod{}, errors.New("period_days 必须在 1 到 3650 之间")
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil || !parsed.IsPositive() {
		return normalizedTokenPeriod{}, errors.New("period_limit_value 必须为正数")
	}

	var limit int64
	if unit == "quota" {
		if !parsed.IsInteger() {
			return normalizedTokenPeriod{}, errors.New("quota 周期限额必须为正整数")
		}
		max := decimal.NewFromInt(int64(common.MaxQuota))
		if parsed.GreaterThan(max) {
			return normalizedTokenPeriod{}, fmt.Errorf("period_quota_limit 不能超过 %d", common.MaxQuota)
		}
		limit = parsed.IntPart()
	} else {
		// 金额单位跟随站点额度展示口径，与令牌余额同源：
		// TOKENS 展示时用户填的就是原生额度，其余按管理员配置的展示汇率折算。
		quotaDecimal := parsed
		if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
			displayRate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
			if displayRate <= 0 || math.IsNaN(displayRate) || math.IsInf(displayRate, 0) ||
				common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
				return normalizedTokenPeriod{}, errors.New("当前汇率配置无效")
			}
			quotaDecimal = parsed.
				Div(decimal.NewFromFloat(displayRate)).
				Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		}
		quota, clamp := common.QuotaFromDecimalChecked(quotaDecimal)
		if clamp != nil {
			return normalizedTokenPeriod{}, errors.New("period_limit_value 超出额度上限")
		}
		if quota < 1 || quota > common.MaxQuota {
			return normalizedTokenPeriod{}, fmt.Errorf("period_quota_limit 必须在 1 到 %d 之间", common.MaxQuota)
		}
		limit = int64(quota)
	}
	if limit < 1 || limit > int64(common.MaxQuota) {
		return normalizedTokenPeriod{}, fmt.Errorf("period_quota_limit 必须在 1 到 %d 之间", common.MaxQuota)
	}

	periodDays := req.PeriodDays
	if periodType != common.TokenPeriodTypeDays {
		periodDays = 0
	}
	return normalizedTokenPeriod{periodType: periodType, periodDays: periodDays, limit: limit, unit: unit}, nil
}

func applyTokenPeriodConfig(token *model.Token, config normalizedTokenPeriod, now time.Time) error {
	if token == nil {
		return errors.New("token 为空")
	}
	oldEnabled := token.PeriodLimitEnabled()
	newEnabled := config.periodType != "" && config.limit > 0
	if !newEnabled {
		// Disabling a policy clears all three state columns and canonical config.
		token.PeriodType = ""
		token.PeriodDays = 0
		token.PeriodQuotaLimit = 0
		token.PeriodLimitUnit = ""
		token.PeriodAnchorAt = 0
		token.PeriodStartAt = 0
		token.PeriodUsedQuota = 0
		return nil
	}

	shapeChanged := !oldEnabled || token.PeriodType != config.periodType || token.PeriodDays != config.periodDays
	token.PeriodType = config.periodType
	token.PeriodDays = config.periodDays
	token.PeriodQuotaLimit = config.limit
	token.PeriodLimitUnit = config.unit
	if shapeChanged {
		anchor := common.TokenPeriodAnchorNow(now)
		start, _, ok := common.TokenPeriodBounds(config.periodType, config.periodDays, anchor, now)
		if !ok {
			return errors.New("period_type 或 period_days 无效")
		}
		token.PeriodAnchorAt = anchor
		token.PeriodStartAt = start
		token.PeriodUsedQuota = 0
	}
	return nil
}

func tokenPeriodConfigNeedsReset(token *model.Token, config normalizedTokenPeriod) bool {
	if config.periodType == "" || config.limit <= 0 {
		return true
	}
	if token == nil || !token.PeriodLimitEnabled() {
		return true
	}
	return token.PeriodType != config.periodType || token.PeriodDays != config.periodDays
}

func enrichTokenPeriodResponse(token *model.Token) {
	if token == nil {
		return
	}
	if !token.PeriodLimitEnabled() {
		token.PeriodResetAt = 0
		token.PeriodRemainingQuota = 0
		token.PeriodUsedQuota = 0
		return
	}
	state := model.TokenPeriodState{
		Type:      token.PeriodType,
		Days:      token.PeriodDays,
		Limit:     token.PeriodQuotaLimit,
		Unit:      token.PeriodLimitUnit,
		AnchorAt:  token.PeriodAnchorAt,
		StartAt:   token.PeriodStartAt,
		UsedQuota: token.PeriodUsedQuota,
	}
	now := time.Now()
	effectiveUsed := state.EffectiveUsed(now)
	token.PeriodUsedQuota = effectiveUsed
	token.PeriodResetAt = state.ResetAt(now)
	token.PeriodRemainingQuota = token.PeriodQuotaLimit - effectiveUsed
	if token.PeriodRemainingQuota < 0 {
		token.PeriodRemainingQuota = 0
	}
}

func buildMaskedTokenResponse(token *model.Token) *model.Token {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	enrichTokenPeriodResponse(&maskedToken)
	return &maskedToken
}

func buildMaskedTokenResponses(tokens []*model.Token) []*model.Token {
	maskedTokens := make([]*model.Token, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        token.RemainQuota + token.UsedQuota,
			"total_used":           token.UsedQuota,
			"total_available":      token.RemainQuota,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	request := dto.TokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		tokenBadRequest(c, err)
		return
	}
	if len(request.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	// 非无限额度时，检查额度值是否超出有效范围
	if !request.UnlimitedQuota {
		if request.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if request.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	periodConfig, err := normalizeTokenPeriod(request)
	if err != nil {
		tokenBadRequest(c, err)
		return
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               request.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        request.ExpiredTime,
		RemainQuota:        request.RemainQuota,
		UnlimitedQuota:     request.UnlimitedQuota,
		ModelLimitsEnabled: request.ModelLimitsEnabled,
		ModelLimits:        request.ModelLimits,
		AllowIps:           request.AllowIps,
		Group:              request.Group,
		CrossGroupRetry:    request.CrossGroupRetry,
	}
	if err := applyTokenPeriodConfig(&cleanToken, periodConfig, time.Now()); err != nil {
		tokenBadRequest(c, err)
		return
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	request := dto.TokenUpdateRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		tokenBadRequest(c, err)
		return
	}
	if len(request.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if !request.UnlimitedQuota {
		if request.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if request.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	cleanToken, err := model.GetTokenByIds(request.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	periodStateReset := false
	periodFieldsPresent := request.PeriodType != nil || request.PeriodDays != nil ||
		request.PeriodLimitUnit != nil || request.PeriodLimitValue != nil
	if statusOnly != "" {
		// status_only is intentionally isolated from period configuration and
		// counters, including malformed client-supplied period fields.
		cleanToken.Status = request.Status
	} else {
		if periodFieldsPresent {
			if request.PeriodType == nil {
				tokenBadRequest(c, errors.New("period_type 必须与周期限额字段一起提供"))
				return
			}
			periodType := strings.TrimSpace(*request.PeriodType)
			if periodType != "" && (request.PeriodLimitUnit == nil || request.PeriodLimitValue == nil ||
				(periodType == common.TokenPeriodTypeDays && request.PeriodDays == nil)) {
				tokenBadRequest(c, errors.New("周期限额字段必须完整提供"))
				return
			}

			// Materialize the update-only pointer patch into the shared request
			// shape so all period validation remains in normalizeTokenPeriod.
			periodRequest := request.TokenRequest
			periodRequest.PeriodType = *request.PeriodType
			if request.PeriodDays != nil {
				periodRequest.PeriodDays = *request.PeriodDays
			} else {
				periodRequest.PeriodDays = 0
			}
			if request.PeriodLimitUnit != nil {
				periodRequest.PeriodLimitUnit = *request.PeriodLimitUnit
			} else {
				periodRequest.PeriodLimitUnit = ""
			}
			if request.PeriodLimitValue != nil {
				periodRequest.PeriodLimitValue = *request.PeriodLimitValue
			} else {
				periodRequest.PeriodLimitValue = ""
			}
			periodConfig, periodErr := normalizeTokenPeriod(periodRequest)
			if periodErr != nil {
				tokenBadRequest(c, periodErr)
				return
			}
			periodStateReset = tokenPeriodConfigNeedsReset(cleanToken, periodConfig)
			if periodErr := applyTokenPeriodConfig(cleanToken, periodConfig, time.Now()); periodErr != nil {
				tokenBadRequest(c, periodErr)
				return
			}
		}
		// If you add more fields, please also update token.Update()
		cleanToken.Name = request.Name
		cleanToken.ExpiredTime = request.ExpiredTime
		cleanToken.RemainQuota = request.RemainQuota
		cleanToken.UnlimitedQuota = request.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = request.ModelLimitsEnabled
		cleanToken.ModelLimits = request.ModelLimits
		cleanToken.AllowIps = request.AllowIps
		cleanToken.Group = request.Group
		cleanToken.CrossGroupRetry = request.CrossGroupRetry
	}
	if statusOnly != "" {
		err = cleanToken.SelectUpdate()
	} else if !periodFieldsPresent {
		err = cleanToken.UpdateWithoutPeriodConfig()
	} else if periodStateReset {
		err = cleanToken.UpdatePeriodConfig()
	} else {
		err = cleanToken.UpdatePeriodConfigPreserveState()
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
