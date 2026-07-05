package controller

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func invitationCodeErrorKey(err error) string {
	switch {
	case errors.Is(err, model.ErrInvitationCodeRequired):
		return i18n.MsgInvitationCodeRequired
	case errors.Is(err, model.ErrInvitationCodeInvalid):
		return i18n.MsgInvitationCodeInvalid
	case errors.Is(err, model.ErrInvitationCodeExhausted):
		return i18n.MsgInvitationCodeExhausted
	case errors.Is(err, model.ErrInvitationCodeExpired):
		return i18n.MsgInvitationCodeExpired
	case errors.Is(err, model.ErrInvitationCodeDisabled):
		return i18n.MsgInvitationCodeDisabled
	case errors.Is(err, model.ErrInvitationCodeQuotaExceeded):
		return i18n.MsgInvitationCodeQuotaExceeded
	case errors.Is(err, model.ErrInvitationCodeAccountTooYoung):
		return i18n.MsgInvitationCodeAccountTooYoung
	case errors.Is(err, model.ErrInvitationCodeUserGenerateDisabled):
		return i18n.MsgInvitationCodeUserGenerateDisabled
	default:
		return ""
	}
}

func handleInvitationCodeError(c *gin.Context, err error) bool {
	if key := invitationCodeErrorKey(err); key != "" {
		common.ApiErrorI18n(c, key)
		return true
	}
	return false
}

type invitationCodeQuotaResponse struct {
	Limit                int    `json:"limit"`
	Used                 int64  `json:"used"`
	Remaining            int    `json:"remaining"`
	AccountAgeDays       int    `json:"account_age_days"`
	MinAccountAgeDays    int    `json:"min_account_age_days"`
	DefaultCodeMaxUses   int    `json:"default_code_max_uses"`
	DefaultCodeValidDays int    `json:"default_code_valid_days"`
	CanGenerate          bool   `json:"can_generate"`
	Reason               string `json:"reason,omitempty"`
}

func buildInvitationCodeQuotaResponse(c *gin.Context, user *model.User) (*invitationCodeQuotaResponse, error) {
	quotaInfo, err := model.GetUserInvitationCodeQuotaInfo(user)
	if err != nil {
		return nil, err
	}
	canGenerateErr := model.CanUserGenerateInvitationCode(user, quotaInfo)
	resp := &invitationCodeQuotaResponse{
		Limit:                quotaInfo.Limit,
		Used:                 quotaInfo.Used,
		Remaining:            quotaInfo.Remaining,
		AccountAgeDays:       quotaInfo.AccountAgeDays,
		MinAccountAgeDays:    quotaInfo.MinAccountAgeDays,
		DefaultCodeMaxUses:   quotaInfo.DefaultCodeMaxUses,
		DefaultCodeValidDays: quotaInfo.DefaultCodeValidDays,
		CanGenerate:          canGenerateErr == nil,
	}
	if canGenerateErr != nil {
		if key := invitationCodeErrorKey(canGenerateErr); key != "" {
			resp.Reason = i18n.T(c, key)
		}
	}
	return resp, nil
}

func validateInvitationCodeExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgInvitationCodeExpired)
	}
	return true, ""
}

func GetAllInvitationCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	invitationCodes, total, err := model.GetAllInvitationCodes(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(invitationCodes)
	common.ApiSuccess(c, pageInfo)
}

func SearchInvitationCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	invitationCodes, total, err := model.SearchInvitationCodes(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(invitationCodes)
	common.ApiSuccess(c, pageInfo)
}

func GetInvitationCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	invitationCode, err := model.GetInvitationCodeById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, invitationCode)
}

func GetInvitationCodeUsages(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	usages, err := model.GetInvitationCodeUsages(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usages)
}

func AddInvitationCode(c *gin.Context) {
	invitationCode := model.InvitationCode{}
	if err := common.DecodeJson(c.Request.Body, &invitationCode); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	invitationCode.Name = strings.TrimSpace(invitationCode.Name)
	if utf8.RuneCountInString(invitationCode.Name) == 0 || utf8.RuneCountInString(invitationCode.Name) > 64 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeNameLength)
		return
	}
	if invitationCode.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if invitationCode.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeCountMax)
		return
	}
	if invitationCode.MaxUses < 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if valid, msg := validateInvitationCodeExpiredTime(c, invitationCode.ExpiredTime); !valid {
		common.ApiErrorMsg(c, msg)
		return
	}
	if invitationCode.OwnerUserId != 0 {
		if _, err := model.GetUserById(invitationCode.OwnerUserId, false); err != nil {
			common.ApiErrorI18n(c, i18n.MsgUserNotExists)
			return
		}
	}
	keys := make([]string, 0, invitationCode.Count)
	for i := 0; i < invitationCode.Count; i++ {
		cleanInvitationCode := model.InvitationCode{
			Name:        invitationCode.Name,
			Code:        strings.ToUpper(common.GetRandomString(12)),
			Status:      model.InvitationCodeStatusEnabled,
			MaxUses:     invitationCode.MaxUses,
			CreatedBy:   c.GetInt("id"),
			OwnerUserId: invitationCode.OwnerUserId,
			CreatedTime: common.GetTimestamp(),
			ExpiredTime: invitationCode.ExpiredTime,
			IsAdmin:     true,
		}
		if err := cleanInvitationCode.Insert(); err != nil {
			common.ApiErrorI18n(c, i18n.MsgInvitationCodeCreateFailed)
			return
		}
		keys = append(keys, cleanInvitationCode.Code)
	}
	common.ApiSuccess(c, keys)
}

func UpdateInvitationCode(c *gin.Context) {
	invitationCode := model.InvitationCode{}
	if err := common.DecodeJson(c.Request.Body, &invitationCode); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	cleanInvitationCode, err := model.GetInvitationCodeById(invitationCode.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	invitationCode.Name = strings.TrimSpace(invitationCode.Name)
	if utf8.RuneCountInString(invitationCode.Name) == 0 || utf8.RuneCountInString(invitationCode.Name) > 64 {
		common.ApiErrorI18n(c, i18n.MsgInvitationCodeNameLength)
		return
	}
	if invitationCode.Status != model.InvitationCodeStatusEnabled && invitationCode.Status != model.InvitationCodeStatusDisabled {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if invitationCode.MaxUses < 0 || (invitationCode.MaxUses > 0 && invitationCode.MaxUses < cleanInvitationCode.UsedCount) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if valid, msg := validateInvitationCodeExpiredTime(c, invitationCode.ExpiredTime); !valid {
		common.ApiErrorMsg(c, msg)
		return
	}
	if invitationCode.OwnerUserId != 0 {
		if _, err := model.GetUserById(invitationCode.OwnerUserId, false); err != nil {
			common.ApiErrorI18n(c, i18n.MsgUserNotExists)
			return
		}
	}
	cleanInvitationCode.Name = invitationCode.Name
	cleanInvitationCode.Status = invitationCode.Status
	cleanInvitationCode.MaxUses = invitationCode.MaxUses
	cleanInvitationCode.ExpiredTime = invitationCode.ExpiredTime
	cleanInvitationCode.OwnerUserId = invitationCode.OwnerUserId
	if err := cleanInvitationCode.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cleanInvitationCode)
}

func DeleteInvitationCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidId)
		return
	}
	if err := model.DeleteInvitationCodeById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DeleteInvalidInvitationCodes(c *gin.Context) {
	rows, err := model.DeleteInvalidInvitationCodes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func GetMyInvitationCodes(c *gin.Context) {
	codes, err := model.GetOwnedInvitationCodesByUserId(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, codes)
}

func GetMyInvitationCodeQuota(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quotaInfo, err := buildInvitationCodeQuotaResponse(c, user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, quotaInfo)
}

func GenerateMyInvitationCode(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	invitationCode, err := model.GenerateUserInvitationCode(user)
	if err != nil {
		if handleInvitationCodeError(c, err) {
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, invitationCode)
}
