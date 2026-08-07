package common

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net/smtp"
	"slices"
	"strings"
	"time"
)

// base64BodyLineWidth 是 RFC 2045 §6.8 建议的 base64 编码行宽度，远低于
// RFC 5321 §4.5.3.1.6 的 998 octet 上限。
const base64BodyLineWidth = 76

// headerFoldWidth 是 RFC 5322 §2.1.1 建议的 header 行宽度。
const headerFoldWidth = 78

// foldHeaderLine 按 RFC 5322 §2.2.3 在空白处折叠 header。
//
// mime.BEncoding.Encode 在内容超长时会切成多段 encoded-word，但**只用空格连接、
// 不做折行**，255 个中文的工单主题因此仍会产出 1200+ octet 的单行。只有含
// encoded-word 的值才需要折叠：纯 ASCII 值里的空格是内容语义，不折以免被规范化。
func foldHeaderLine(name string, value string) string {
	line := name + ": " + value
	if len(line) <= headerFoldWidth || !strings.Contains(value, "=?") {
		return line + "\r\n"
	}
	var sb strings.Builder
	current := name + ":"
	for _, token := range strings.Split(value, " ") {
		if current != name+":" && len(current)+1+len(token) > headerFoldWidth {
			sb.WriteString(current + "\r\n")
			current = ""
		}
		current += " " + token
	}
	sb.WriteString(current + "\r\n")
	return sb.String()
}

func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func shouldUseSMTPLoginAuth() bool {
	if SMTPForceAuthLogin {
		return true
	}
	return isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}

func getSMTPAuth() smtp.Auth {
	return AutoSMTPAuth(SMTPAccount, SMTPToken)
}

func shouldAuthenticateSMTP() bool {
	return SMTPAccount != "" && SMTPToken != ""
}

func smtpTLSConfig() *tls.Config {
	return &tls.Config{
		ServerName:         SMTPServer,
		InsecureSkipVerify: SMTPInsecureSkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

func newSMTPClient(addr string) (*smtp.Client, error) {
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		conn, err := tls.Dial("tcp", addr, smtpTLSConfig())
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}

	if SMTPStartTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

// buildMailMessage 组装完整的 RFC 5322 报文。
//
// 正文一律以 base64 编码并按 76 列折叠，主题与发件人显示名走 RFC 2047 encoded-word
// （stdlib 在超长时自动分片折叠），保证报文任意一行都不超过 RFC 5321 §4.5.3.1.6 的
// 998 octet 上限。邮件模板渲染出的信息表是零换行的单行 HTML，动辄 2000+ octet，
// 不做编码会被 MTA 以 "500 Line too long" 整封拒收。
//
// base64 输出字符集恒为 A-Za-z0-9+/=，永不出现行首点，与 textproto.DotWriter 的
// dot-stuffing 不存在双重转义。
func buildMailMessage(subject string, receiver string, content string) ([]byte, error) {
	messageID, err := generateMessageID()
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "To: %s\r\n", receiver)
	sb.WriteString(foldHeaderLine("From", mime.BEncoding.Encode("UTF-8", SystemName)+" <"+SMTPFrom+">"))
	sb.WriteString(foldHeaderLine("Subject", mime.BEncoding.Encode("UTF-8", subject)))
	fmt.Fprintf(&sb, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&sb, "Message-ID: %s\r\n", messageID)
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")

	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	for start := 0; start < len(encoded); start += base64BodyLineWidth {
		end := min(start+base64BodyLineWidth, len(encoded))
		sb.WriteString(encoded[start:end])
		sb.WriteString("\r\n")
	}
	return []byte(sb.String()), nil
}

func SendEmail(subject string, receiver string, content string) error {
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	mail, err := buildMailMessage(subject, receiver, content)
	if err != nil {
		return err
	}
	auth := getSMTPAuth()
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	to := strings.Split(receiver, ";")
	client, err := newSMTPClient(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldAuthenticateSMTP() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, receiver := range to {
		if err = client.Rcpt(receiver); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(mail)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	err = client.Quit()
	if err != nil {
		SysError(fmt.Sprintf("failed to send email to %s: %v", receiver, err))
	}
	return err
}
