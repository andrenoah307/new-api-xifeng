package service

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const ticketContentPreviewMaxLen = 500

func ticketStatusLabel(status int) string {
	switch status {
	case model.TicketStatusOpen:
		return "待处理"
	case model.TicketStatusProcessing:
		return "处理中"
	case model.TicketStatusResolved:
		return "已解决"
	case model.TicketStatusClosed:
		return "已关闭"
	default:
		return "未知"
	}
}

func ticketTypeLabel(ticketType string) string {
	switch ticketType {
	case model.TicketTypeGeneral:
		return "普通工单"
	case model.TicketTypeRefund:
		return "退款申请"
	case model.TicketTypeInvoice:
		return "发票申请"
	default:
		return ticketType
	}
}

func ticketPriorityLabel(priority int) string {
	switch priority {
	case 1:
		return "低"
	case 3:
		return "高"
	default:
		return "中"
	}
}

// ticketContentPreview 返回截断后、已 HTML 转义的内容预览
func ticketContentPreview(content string) string {
	trimmed := strings.TrimSpace(content)
	if len([]rune(trimmed)) > ticketContentPreviewMaxLen {
		runes := []rune(trimmed)
		trimmed = string(runes[:ticketContentPreviewMaxLen]) + "..."
	}
	return common.EscapeAndBreak(trimmed)
}

func buildTicketIntro(ticket *model.Ticket, message *model.TicketMessage, isAdmin bool) string {
	if isAdmin {
		return fmt.Sprintf("来自 %s 的一条新工单，等你看看。", strings.TrimSpace(ticket.Username))
	}
	if message != nil {
		return "我们刚刚更新了你的工单，以下是最新进展。"
	}
	return "这是你工单的最新进展。"
}

func ticketLink() string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/ticket", base)
}

// buildTicketVars 准备工单邮件的所有占位变量。所有 value 都已转义为安全 HTML。
func buildTicketVars(ticket *model.Ticket, message *model.TicketMessage, isAdmin bool, heading string) map[string]string {
	createdAt := time.Unix(ticket.CreatedTime, 0).Format("2006-01-02 15:04:05")
	typeLabel := html.EscapeString(ticketTypeLabel(ticket.Type))
	priorityLabel := html.EscapeString(ticketPriorityLabel(ticket.Priority))
	statusLabel := html.EscapeString(ticketStatusLabel(ticket.Status))
	subjectEsc := html.EscapeString(ticket.Subject)
	usernameEsc := html.EscapeString(strings.TrimSpace(ticket.Username))
	createdEsc := html.EscapeString(createdAt)

	rows := []common.EmailTemplateRow{
		{Label: "工单编号", Value: fmt.Sprintf("#%d", ticket.Id)},
		{Label: "主题", Value: subjectEsc},
		{Label: "类型", Value: typeLabel},
		{Label: "优先级", Value: priorityLabel},
		{Label: "当前状态", Value: statusLabel},
		{Label: "提交用户", Value: usernameEsc},
		{Label: "创建时间", Value: createdEsc},
	}

	replyUsername := ""
	replyTime := ""
	previewHTML := ""
	if message != nil {
		replyUsername = strings.TrimSpace(message.Username)
		replyTime = time.Unix(message.CreatedTime, 0).Format("2006-01-02 15:04:05")
		rows = append(rows, common.EmailTemplateRow{
			Label: "最新回复",
			Value: fmt.Sprintf("%s · %s", html.EscapeString(replyUsername), html.EscapeString(replyTime)),
		})
		previewHTML = ticketContentPreview(message.Content)
	}

	actionLabel := "查看工单"
	if isAdmin {
		actionLabel = "前往处理"
	}

	return map[string]string{
		"system_name":           html.EscapeString(common.SystemNameOrDefault()),
		"server_address":        html.EscapeString(strings.TrimRight(system_setting.ServerAddress, "/")),
		"heading":               html.EscapeString(heading),
		"intro":                 html.EscapeString(buildTicketIntro(ticket, message, isAdmin)),
		"ticket_id":             fmt.Sprintf("%d", ticket.Id),
		"ticket_subject":        subjectEsc,
		"ticket_type":           typeLabel,
		"ticket_priority":       priorityLabel,
		"ticket_status":         statusLabel,
		"ticket_username":       usernameEsc,
		"ticket_created_at":     createdEsc,
		"reply_username":        html.EscapeString(replyUsername),
		"reply_time":            html.EscapeString(replyTime),
		"info_table":            common.RenderInfoTableHTML(rows),
		"content_preview":       previewHTML,
		"content_preview_block": common.RenderPreviewBlockHTML("内容预览", previewHTML),
		"action_url":            html.EscapeString(ticketLink()),
		"action_label":          html.EscapeString(actionLabel),
	}
}

// NotifyTicketCreatedToAdmin 异步通知管理员：用户创建了新工单
func NotifyTicketCreatedToAdmin(ticket *model.Ticket, message *model.TicketMessage) {
	if ticket == nil {
		return
	}
	if !common.TicketNotifyEnabled {
		return
	}
	recipients := parseAdminEmails(common.TicketAdminEmail)
	if len(recipients) == 0 {
		return
	}
	gopool.Go(func() {
		vars := buildTicketVars(ticket, message, true, "新工单")
		subject, body := RenderEmailByKey(constant.EmailTemplateKeyTicketCreatedAdmin, vars)
		if subject == "" || body == "" {
			return
		}
		for _, to := range recipients {
			if err := common.SendEmail(subject, to, body); err != nil {
				common.SysLog(fmt.Sprintf("failed to send ticket-created email to admin %s (ticket=%d): %s", to, ticket.Id, err.Error()))
			}
		}
	})
}

// NotifyTicketReplyToUser 异步通知用户：管理员回复了工单
func NotifyTicketReplyToUser(ticket *model.Ticket, message *model.TicketMessage) {
	if ticket == nil || message == nil {
		return
	}
	if !common.TicketNotifyEnabled {
		return
	}
	gopool.Go(func() {
		user, err := model.GetUserById(ticket.UserId, false)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to load user %d for ticket reply email (ticket=%d): %s", ticket.UserId, ticket.Id, err.Error()))
			return
		}
		userSetting := user.GetSetting()
		userEmail := ResolveUserNotificationEmail(user, userSetting)
		if userEmail == "" {
			return
		}
		vars := buildTicketVars(ticket, message, false, "你的工单有新回复")
		subject, body := RenderEmailByKey(constant.EmailTemplateKeyTicketReplyUser, vars)
		if subject == "" || body == "" {
			return
		}
		notify := dto.NewNotify(dto.NotifyTypeTicketReply, subject, body, nil)
		// 走统一的限流 + 发送通道
		if err := NotifyUser(user.Id, userEmail, userSetting, notify); err != nil {
			common.SysLog(fmt.Sprintf("failed to send ticket reply notification to user %d (ticket=%d): %s", user.Id, ticket.Id, err.Error()))
		}
	})
}

func parseAdminEmails(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// 支持 ; , 空白 换行作为分隔
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	emails := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		e := strings.TrimSpace(f)
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		emails = append(emails, e)
	}
	return emails
}

