package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// GetEmailTemplate 读取保存的自定义主题/正文；若 OptionMap 中对应 key 为空，回落到 spec 默认值。
//
// 返回 subject / body 都是未渲染的模板字符串（仍含 {{var}} 占位）。
func GetEmailTemplate(key string) (subject string, body string, spec constant.EmailTemplateSpec, ok bool) {
	spec, ok = constant.FindEmailTemplateSpec(key)
	if !ok {
		return "", "", spec, false
	}

	common.OptionMapRWMutex.RLock()
	savedSubject := common.OptionMap[constant.EmailTemplateSubjectKey(key)]
	savedBody := common.OptionMap[constant.EmailTemplateBodyKey(key)]
	common.OptionMapRWMutex.RUnlock()

	subject = savedSubject
	if subject == "" {
		subject = spec.DefaultSubject
	}
	body = savedBody
	if body == "" {
		body = spec.DefaultBody
	}
	return subject, body, spec, true
}

// RenderEmailByKey 读取模板并用 vars 渲染。返回 (subject, body)。
// 若 key 不存在，返回两个空串 —— 调用方应先判断。
func RenderEmailByKey(key string, vars map[string]string) (string, string) {
	subject, body, _, ok := GetEmailTemplate(key)
	if !ok {
		return "", ""
	}
	return common.RenderPlaceholders(subject, vars), common.RenderPlaceholders(body, vars)
}

// PreviewEmailTemplate 用 spec.Variables 中的 Sample 作为占位变量，渲染一份预览。
//
// 传入的 subject/body 若为空则使用已保存（或默认）的模板 —— 主要用于"未保存先预览"。
func PreviewEmailTemplate(key, subject, body string) (renderedSubject, renderedBody string, err error) {
	spec, ok := constant.FindEmailTemplateSpec(key)
	if !ok {
		return "", "", fmt.Errorf("unknown email template key: %s", key)
	}

	if subject == "" || body == "" {
		savedSubject, savedBody, _, _ := GetEmailTemplate(key)
		if subject == "" {
			subject = savedSubject
		}
		if body == "" {
			body = savedBody
		}
	}

	vars := sampleVarsFromSpec(spec)
	return common.RenderPlaceholders(subject, vars), common.RenderPlaceholders(body, vars), nil
}

func sampleVarsFromSpec(spec constant.EmailTemplateSpec) map[string]string {
	vars := make(map[string]string, len(spec.Variables))
	for _, v := range spec.Variables {
		vars[v.Name] = v.Sample
	}
	return vars
}

// SendEmailTemplateTest 把编辑中的模板按示例变量渲染后发到**调用管理员本人**的邮箱，
// 供管理员自验 SMTP 配置与报文编码是否正常。
//
// 收件人固定为调用者（与 SendEnforcementTestEmail 一致），不接受任意收件人，
// 避免该端点被当成任意邮件转发器；全过程不写库。
func SendEmailTemplateTest(key, subject, body string, adminUserID int) error {
	if adminUserID <= 0 {
		return fmt.Errorf("无效的管理员账号")
	}
	renderedSubject, renderedBody, err := PreviewEmailTemplate(key, subject, body)
	if err != nil {
		return err
	}
	user, err := model.GetUserById(adminUserID, false)
	if err != nil {
		return err
	}
	email := strings.TrimSpace(user.Email)
	if email == "" {
		return fmt.Errorf("当前账号未绑定邮箱，无法接收测试邮件")
	}
	return common.SendEmail("[测试] "+renderedSubject, email, renderedBody)
}
