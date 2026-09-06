package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// ErrNotificationLimitExceeded 表示通知被 CheckNotificationLimit 拒绝：近期已经发过同类型通知，不是投递失败。
var ErrNotificationLimitExceeded = errors.New("notification limit exceeded")

// ResolveUserNotificationEmail 返回用户收件邮箱：优先用 UserSetting.NotificationEmail，其次 user.Email。
// 两个都空则返回空串。
func ResolveUserNotificationEmail(user *model.User, userSetting dto.UserSetting) string {
	if override := strings.TrimSpace(userSetting.NotificationEmail); override != "" {
		return override
	}
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.Email)
}

func NotifyRootUser(t string, subject string, content string) {
	user := model.GetRootUser().ToBaseUser()
	err := NotifyUser(user.Id, user.Email, user.GetSetting(), dto.NewNotify(t, subject, content, nil))
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to notify root user: %s", err.Error()))
	}
}

func NotifyUpstreamModelUpdateWatchers(subject string, content string) {
	var users []model.User
	if err := model.DB.
		Select("id", "email", "role", "status", "setting").
		Where("status = ? AND role >= ?", common.UserStatusEnabled, common.RoleAdminUser).
		Find(&users).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to query upstream update notification users: %s", err.Error()))
		return
	}

	notification := dto.NewNotify(dto.NotifyTypeChannelUpdate, subject, content, nil)
	sentCount := 0
	for _, user := range users {
		userSetting := user.GetSetting()
		if !userSetting.UpstreamModelUpdateNotifyEnabled {
			continue
		}
		if err := NotifyUser(user.Id, user.Email, userSetting, notification); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify user %d for upstream model update: %s", user.Id, err.Error()))
			continue
		}
		sentCount++
	}
	common.SysLog(fmt.Sprintf("upstream model update notifications sent: %d", sentCount))
}

func NotifyUser(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
	notifyType := userSetting.NotifyType
	if notifyType == "" {
		notifyType = dto.NotifyTypeEmail
	}

	// Check notification limit
	canSend, err := CheckNotificationLimit(userId, data.Type)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check notification limit: %s", err.Error()))
		return err
	}
	if !canSend {
		return fmt.Errorf("%w for user %d with type %s", ErrNotificationLimitExceeded, userId, data.Type)
	}

	switch notifyType {
	case dto.NotifyTypeEmail:
		// 优先使用设置中的通知邮箱，如果为空则使用用户的默认邮箱
		emailToUse := userSetting.NotificationEmail
		if emailToUse == "" {
			emailToUse = userEmail
		}
		if emailToUse == "" {
			common.SysLog(fmt.Sprintf("user %d has no email, skip sending email", userId))
			return nil
		}
		return sendEmailNotify(emailToUse, data)
	case dto.NotifyTypeWebhook:
		webhookURLStr := userSetting.WebhookUrl
		if webhookURLStr == "" {
			common.SysLog(fmt.Sprintf("user %d has no webhook url, skip sending webhook", userId))
			return nil
		}

		// 获取 webhook secret
		webhookSecret := userSetting.WebhookSecret
		return SendWebhookNotify(webhookURLStr, webhookSecret, data)
	case dto.NotifyTypeBark:
		barkURL := userSetting.BarkUrl
		if barkURL == "" {
			common.SysLog(fmt.Sprintf("user %d has no bark url, skip sending bark", userId))
			return nil
		}
		return sendBarkNotify(barkURL, data)
	case dto.NotifyTypeGotify:
		gotifyUrl := userSetting.GotifyUrl
		gotifyToken := userSetting.GotifyToken
		if gotifyUrl == "" || gotifyToken == "" {
			common.SysLog(fmt.Sprintf("user %d has no gotify url or token, skip sending gotify", userId))
			return nil
		}
		return sendGotifyNotify(gotifyUrl, gotifyToken, userSetting.GotifyPriority, data)
	}
	return nil
}

func sendEmailNotify(userEmail string, data dto.Notify) error {
	content := renderNotifyContent(data.Content, data.Values)
	return common.SendEmail(data.Title, userEmail, content)
}

// renderNotifyContent 按顺序把 content 里的 {{value}} 占位符替换为 values 中的值。
// 四个通知渠道共用同一份实现：webhook 曾经误用 fmt.Sprintf，对 {{value}} 模板无效，
// 用户收到的正文里占位符原样保留并被追加 %!(EXTRA ...) 噪声。
func renderNotifyContent(content string, values []interface{}) string {
	for _, value := range values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}
	return content
}

// notifyRequestTimeout 通知外呼的整体超时。通知不是中继流量，10 秒足够；
// 必须有界，否则一个不返回的对端会永久占住结算后派生的协程。
const notifyRequestTimeout = 10 * time.Second

// notifyResponseBodyLimit 读取响应体的上限，兼作失败诊断信息的长度上限。
const notifyResponseBodyLimit = 4 << 10

// doNotifyRequest 是 webhook / bark / gotify 共用的外呼实现，统一了 Worker/直连分支、
// SSRF 校验、超时与响应处理。body 为空表示无体请求（如 Bark 的 GET）。
// workerOnlyHeaders 仅在 Worker 分支追加：webhook 的 Authorization 头历来只走 Worker，
// 直连路径不能带上它，否则明文密钥会被发到用户配置的 http:// 地址。
func doNotifyRequest(channel string, method string, targetURL string, headers map[string]string, workerOnlyHeaders map[string]string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), notifyRequestTimeout)
	defer cancel()

	var resp *http.Response
	var err error
	if system_setting.EnableWorker() {
		workerHeaders := make(map[string]string, len(headers)+len(workerOnlyHeaders))
		for key, value := range headers {
			workerHeaders[key] = value
		}
		for key, value := range workerOnlyHeaders {
			workerHeaders[key] = value
		}
		workerReq := &WorkerRequest{
			URL:     targetURL,
			Key:     system_setting.WorkerValidKey,
			Method:  method,
			Headers: workerHeaders,
			Body:    body,
		}
		resp, err = DoWorkerRequestWithContext(ctx, workerReq)
		if err != nil {
			return fmt.Errorf("failed to send %s request through worker: %v", channel, err)
		}
	} else {
		if err := ValidateSSRFProtectedFetchURL(targetURL); err != nil {
			return fmt.Errorf("request reject: %v", err)
		}

		var reqBody io.Reader
		if len(body) > 0 {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, targetURL, reqBody)
		if err != nil {
			return fmt.Errorf("failed to create %s request: %v", channel, err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err = GetSSRFProtectedHTTPClient().Do(req)
		if err != nil {
			return fmt.Errorf("failed to send %s request: %v", channel, err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, notifyResponseBodyLimit))
		detail := strings.Join(strings.Fields(string(raw)), " ")
		if detail == "" {
			detail = "empty response body"
		}
		return fmt.Errorf("%s request failed with status code %d: %s", channel, resp.StatusCode, detail)
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, notifyResponseBodyLimit))
	return nil
}

func sendBarkNotify(barkURL string, data dto.Notify) error {
	content := renderNotifyContent(data.Content, data.Values)

	// 替换模板变量
	finalURL := strings.ReplaceAll(barkURL, "{{title}}", url.QueryEscape(data.Title))
	finalURL = strings.ReplaceAll(finalURL, "{{content}}", url.QueryEscape(content))

	headers := map[string]string{"User-Agent": "OneAPI-Bark-Notify/1.0"}
	return doNotifyRequest("bark", http.MethodGet, finalURL, headers, nil, nil)
}

func sendGotifyNotify(gotifyUrl string, gotifyToken string, priority int, data dto.Notify) error {
	content := renderNotifyContent(data.Content, data.Values)

	// 构建完整的 Gotify API URL
	// 确保 URL 以 /message 结尾
	finalURL := strings.TrimSuffix(gotifyUrl, "/") + "/message?token=" + url.QueryEscape(gotifyToken)

	// Gotify优先级范围0-10，如果超出范围则使用默认值5
	if priority < 0 || priority > 10 {
		priority = 5
	}

	// 构建 JSON payload
	type GotifyMessage struct {
		Title    string `json:"title"`
		Message  string `json:"message"`
		Priority int    `json:"priority"`
	}

	payload := GotifyMessage{
		Title:    data.Title,
		Message:  content,
		Priority: priority,
	}

	// 序列化为 JSON
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal gotify payload: %v", err)
	}

	// Use the direct-path User-Agent for both paths intentionally, correcting historical drift.
	headers := map[string]string{
		"Content-Type": "application/json; charset=utf-8",
		"User-Agent":   "NewAPI-Gotify-Notify/1.0",
	}
	return doNotifyRequest("gotify", http.MethodPost, finalURL, headers, nil, payloadBytes)
}
