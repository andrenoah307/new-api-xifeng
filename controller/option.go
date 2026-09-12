package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

func isPaymentComplianceOptionKey(key string) bool {
	return strings.HasPrefix(key, "payment_setting.compliance_")
}

func isPositiveOptionValue(value string) bool {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return intValue > 0
	}
	floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && floatValue > 0
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		isSensitiveKey := strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			strings.HasSuffix(k, "Key") ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key")
		if isSensitiveKey {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func applyOptionUpdate(c *gin.Context, key string, value any) error {
	option := OptionUpdateRequest{Key: key, Value: value}
	var err error
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = common.Interface2String(option.Value)
	}
	switch option.Key {
	case "QuotaForInviter", "QuotaForInvitee":
		if isPositiveOptionValue(option.Value.(string)) && !operation_setting.IsPaymentComplianceConfirmed() {
			return errors.New(common.TranslateMessage(c, i18n.MsgPaymentComplianceRequired))
		}
	default:
		if isPaymentComplianceOptionKey(option.Key) {
			return errors.New("合规确认字段不允许通过通用设置接口修改")
		}
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			return errors.New("无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！")
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			return errors.New("无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！")
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			return errors.New("无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！")
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			return errors.New("无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！")
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			return errors.New("无法启用邮箱域名限制，请先填入限制的邮箱域名！")
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			return errors.New("无法启用微信登录，请先填入微信登录相关配置信息！")
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			return errors.New("无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！")
		}
	case "InvitationCodePolicy":
		if err := setting.ValidateInvitationCodePolicy(option.Value.(string)); err != nil {
			return errors.New("邀请码策略 JSON 无效: " + err.Error())
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			return errors.New("无法启用 Telegram OAuth，请先填入 Telegram Bot Token！")
		}
	case "theme.frontend":
		if option.Value != "default" && option.Value != "classic" {
			return errors.New("无效的主题值，可选值：default（新版前端）、classic（经典前端）")
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(option.Value.(string))
		if err != nil {
			return errors.New("图片倍率设置失败: " + err.Error())
		}
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string))
		if err != nil {
			return errors.New("音频倍率设置失败: " + err.Error())
		}
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string))
		if err != nil {
			return errors.New("音频补全倍率设置失败: " + err.Error())
		}
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(option.Value.(string))
		if err != nil {
			return errors.New("缓存创建倍率设置失败: " + err.Error())
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "ModelNameRPMRateLimit":
		err = setting.CheckModelNameRPMRateLimit(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "RequestBlacklist":
		err = setting.CheckRequestBlacklist(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			return errors.New(err.Error())
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			return errors.New(err.Error())
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			return errors.New(err.Error())
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			return errors.New(err.Error())
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			return errors.New(err.Error())
		}
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		return err
	}
	return nil
}
func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if err := applyOptionUpdate(c, option.Key, option.Value); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "option.update", map[string]interface{}{"key": option.Key})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

type OptionBatchUpdateRequest struct {
	Options []OptionUpdateRequest `json:"options"`
}

const maxBatchOptionUpdates = 64

func UpdateOptions(c *gin.Context) {
	var request OptionBatchUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if len(request.Options) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "配置项不能为空"})
		return
	}
	if len(request.Options) > maxBatchOptionUpdates {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "配置项数量超过上限"})
		return
	}
	seen := make(map[string]struct{}, len(request.Options))
	for _, option := range request.Options {
		if option.Key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "配置项名称不能为空"})
			return
		}
		if _, ok := seen[option.Key]; ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "配置项名称重复"})
			return
		}
		seen[option.Key] = struct{}{}
	}
	batchID := common.GetUUID()
	applied := make([]string, 0, len(request.Options))
	for _, option := range request.Options {
		if err := applyOptionUpdate(c, option.Key, option.Value); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": gin.H{"applied": applied, "failed": gin.H{"key": option.Key, "message": err.Error()}}})
			return
		}
		applied = append(applied, option.Key)
		recordManageAudit(c, "option.update", map[string]interface{}{"key": option.Key, "batch_id": batchID})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"applied": applied}})
}
