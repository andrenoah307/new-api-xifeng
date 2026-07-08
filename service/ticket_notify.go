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

// 工单状态变更邮件的标题文案。集中在此，便于日后调整或走 i18n。
const (
	TicketStatusReasonGeneric         = "工单状态已更新"
	TicketStatusReasonRefundApproved  = "你的退款申请已通过"
	TicketStatusReasonRefundRejected  = "你的退款申请已被驳回"
	TicketStatusReasonInvoiceIssued   = "发票已开具"
	TicketStatusReasonInvoiceRejected = "发票申请未通过"
)

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
		if message != nil {
			replyBy := strings.TrimSpace(message.Username)
			if replyBy == "" {
				replyBy = strings.TrimSpace(ticket.Username)
			}
			return fmt.Sprintf("%s 在工单中追加了新的回复。", replyBy)
		}
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

// NotifyTicketCreatedToAdmin 异步通知管理员：用户创建了新工单。
// 若工单已完成自动分配，邮件中会附带"已分配给客服 xxx"一行；否则提示"待认领"。
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
		// 叠加分配信息：已分配 -> 显示客服名；未分配 -> "待认领"
		vars["assignee_line"] = html.EscapeString(buildAssigneeLine(ticket))
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

// NotifyTicketAssigned 异步通知被分配到的客服，以及抄送管理员邮箱。
// 触发时机：
//   - 自动分配成功后
//   - 管理员手动指派后（assigneeId 发生变化）
//
// 抄送管理员是为了让整个工单流转对管理员透明，无需额外配置邮箱（客服用自己的邮箱）。
func NotifyTicketAssigned(ticket *model.Ticket, assigneeId int) {
	if ticket == nil || assigneeId <= 0 {
		return
	}
	if !common.TicketNotifyEnabled {
		return
	}
	gopool.Go(func() {
		assignee, err := model.GetUserById(assigneeId, false)
		if err != nil {
			common.SysLog(fmt.Sprintf("ticket assigned notify: failed to load user %d: %s", assigneeId, err.Error()))
			return
		}
		assigneeSetting := assignee.GetSetting()
		assigneeEmail := ResolveUserNotificationEmail(assignee, assigneeSetting)

		heading := "你有一条新的工单待处理"
		vars := buildTicketVars(ticket, nil, true, heading)
		vars["assignee_line"] = html.EscapeString(buildAssigneeLine(ticket))
		vars["intro"] = html.EscapeString(fmt.Sprintf(
			"工单 #%d「%s」已分配给你，请及时处理。",
			ticket.Id, strings.TrimSpace(ticket.Subject)))

		subject, body := RenderEmailByKey(constant.EmailTemplateKeyTicketAssigned, vars)
		if subject == "" || body == "" {
			// 未配置模板时使用最简备用文案，不让通知丢失
			subject = fmt.Sprintf("[%s] 你有一条新的工单待处理 #%d", common.SystemNameOrDefault(), ticket.Id)
			body = fmt.Sprintf(
				`<p>%s</p><p>工单主题：%s</p><p>请前往管理后台查看并处理。</p>`,
				html.EscapeString(vars["intro"]),
				html.EscapeString(strings.TrimSpace(ticket.Subject)))
		}

		if assigneeEmail != "" {
			if err := common.SendEmail(subject, assigneeEmail, body); err != nil {
				common.SysLog(fmt.Sprintf("failed to send ticket-assigned email to assignee %d (ticket=%d): %s",
					assigneeId, ticket.Id, err.Error()))
			}
		}

		// 同时抄送管理员列表，正文里会包含"已分配给客服 xxx"这一行
		recipients := parseAdminEmails(common.TicketAdminEmail)
		for _, to := range recipients {
			if to == assigneeEmail {
				continue
			}
			if err := common.SendEmail(subject, to, body); err != nil {
				common.SysLog(fmt.Sprintf("failed to send ticket-assigned email to admin %s (ticket=%d): %s",
					to, ticket.Id, err.Error()))
			}
		}
	})
}

// buildAssigneeLine 构造 "已分配给客服 xxx" 或 "待认领" 的可读文字。
// 不做 HTML 转义；调用方在写入 vars 时自行 escape。
func buildAssigneeLine(ticket *model.Ticket) string {
	if ticket.AssigneeId <= 0 {
		return "当前状态：待认领"
	}
	assignee, err := model.GetUserById(ticket.AssigneeId, false)
	if err != nil || assignee == nil {
		return fmt.Sprintf("已分配给用户 ID #%d", ticket.AssigneeId)
	}
	name := strings.TrimSpace(assignee.DisplayName)
	if name == "" {
		name = strings.TrimSpace(assignee.Username)
	}
	if name == "" {
		name = fmt.Sprintf("#%d", assignee.Id)
	}
	roleLabel := common.RoleLabel(assignee.Role)
	return fmt.Sprintf("已分配给%s %s", roleLabel, name)
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

// NotifyTicketReplyToAdmin 异步通知管理员：用户在已有工单中追加了回复
func NotifyTicketReplyToAdmin(ticket *model.Ticket, message *model.TicketMessage) {
	if ticket == nil || message == nil {
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
		vars := buildTicketVars(ticket, message, true, "工单有新回复")
		subject, body := RenderEmailByKey(constant.EmailTemplateKeyTicketReplyAdmin, vars)
		if subject == "" || body == "" {
			return
		}
		for _, to := range recipients {
			if err := common.SendEmail(subject, to, body); err != nil {
				common.SysLog(fmt.Sprintf("failed to send ticket-reply email to admin %s (ticket=%d): %s", to, ticket.Id, err.Error()))
			}
		}
	})
}

// buildStatusChangeDesc 构造 "旧状态 → 新状态" 的描述；若 prevStatus <= 0 或与当前相同，则只显示当前状态。
func buildStatusChangeDesc(prevStatus, curStatus int) string {
	cur := ticketStatusLabel(curStatus)
	if prevStatus <= 0 || prevStatus == curStatus {
		return cur
	}
	return fmt.Sprintf("%s → %s", ticketStatusLabel(prevStatus), cur)
}

// NotifyIfTicketStatusChanged 仅在 prevStatus != ticket.Status 时发出状态变更邮件；否则静默返回。
// 用于控制器在每种状态推进（手动调整 / 发票处理 / 退款处理）后统一触达用户，避免重复模板。
func NotifyIfTicketStatusChanged(ticket *model.Ticket, prevStatus int, reason string) {
	if ticket == nil || prevStatus == ticket.Status {
		return
	}
	NotifyTicketStatusChangedToUser(ticket, prevStatus, reason)
}

// NotifyTicketStatusChangedToUser 异步通知用户：工单状态发生变更（手动调整 / 发票处理 / 退款处理）。
//
// prevStatus 为状态变更前的工单主状态（Ticket.Status）；若传入 0 或与当前相同，则邮件中不显示 "→"。
// reason 用于区分触发场景，会拼接到邮件抬头，便于用户快速识别是"人工调整"、"发票已开具"还是"退款已到账"等。
func NotifyTicketStatusChangedToUser(ticket *model.Ticket, prevStatus int, reason string) {
	if ticket == nil {
		return
	}
	if !common.TicketNotifyEnabled {
		return
	}
	// 状态未发生实际变更时，不再发送邮件，避免骚扰。
	if prevStatus > 0 && prevStatus == ticket.Status {
		return
	}
	gopool.Go(func() {
		user, err := model.GetUserById(ticket.UserId, false)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to load user %d for ticket status email (ticket=%d): %s", ticket.UserId, ticket.Id, err.Error()))
			return
		}
		userSetting := user.GetSetting()
		userEmail := ResolveUserNotificationEmail(user, userSetting)
		if userEmail == "" {
			return
		}

		heading := "工单状态已更新"
		if r := strings.TrimSpace(reason); r != "" {
			heading = r
		}
		vars := buildTicketVars(ticket, nil, false, heading)
		vars["intro"] = html.EscapeString(fmt.Sprintf("你的工单「%s」状态已更新为「%s」。",
			strings.TrimSpace(ticket.Subject), ticketStatusLabel(ticket.Status)))
		vars["status_change"] = html.EscapeString(buildStatusChangeDesc(prevStatus, ticket.Status))

		subject, body := RenderEmailByKey(constant.EmailTemplateKeyTicketStatusUser, vars)
		if subject == "" || body == "" {
			return
		}
		notify := dto.NewNotify(dto.NotifyTypeTicketStatus, subject, body, nil)
		if err := NotifyUser(user.Id, userEmail, userSetting, notify); err != nil {
			common.SysLog(fmt.Sprintf("failed to send ticket status notification to user %d (ticket=%d): %s", user.Id, ticket.Id, err.Error()))
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

