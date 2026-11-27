package common

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type EmailAttachment struct {
	Name string
	Data []byte
}

func BuildAttachmentMail(subject, receiver, content, from string, files []EmailAttachment) ([]byte, error) {
	for _, address := range []string{receiver, from} {
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
			return nil, errors.New("invalid email address")
		}
	}
	if strings.ContainsAny(subject, "\r\n") || len(files) == 0 || len(files) > 20 {
		return nil, errors.New("invalid attachment message")
	}
	total := 0
	for _, file := range files {
		if len(file.Data) == 0 || len(file.Data) > 10<<20 || len(file.Name) > 512 {
			return nil, errors.New("invalid attachment size")
		}
		total += len(file.Data)
	}
	if total > 10<<20 {
		return nil, errors.New("attachment message too large")
	}
	messageID, err := generateMessageID(from)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	parts := append([]EmailAttachment{{Data: []byte(content)}}, files...)
	for i, file := range parts {
		header := textproto.MIMEHeader{}
		header.Set("Content-Transfer-Encoding", "base64")
		if i == 0 {
			header.Set("Content-Type", "text/html; charset=UTF-8")
		} else {
			if file.Name == "" || strings.ContainsAny(file.Name, "\r\n/\\") {
				return nil, errors.New("invalid attachment name")
			}
			header.Set("Content-Type", "image/png")
			header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, err
		}
		encoded := base64.StdEncoding.EncodeToString(file.Data)
		for start := 0; start < len(encoded); start += base64BodyLineWidth {
			fmt.Fprint(part, encoded[start:min(start+base64BodyLineWidth, len(encoded))]+"\r\n")
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "To: %s\r\n", receiver)
	message.WriteString(foldHeaderLine("From", mime.BEncoding.Encode("UTF-8", SystemName)+" <"+from+">"))
	message.WriteString(foldHeaderLine("Subject", mime.BEncoding.Encode("UTF-8", subject)))
	fmt.Fprintf(&message, "Date: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\n", time.Now().Format(time.RFC1123Z), messageID)
	fmt.Fprintf(&message, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", writer.Boundary())
	message.Write(body.Bytes())
	return message.Bytes(), nil
}

// SendEmailAttachments reports an uncertain result only once DATA transmission
// starts. A successful DATA acknowledgement is delivery even if QUIT fails.
func SendEmailAttachments(subject, receiver, content string, files []EmailAttachment) (uncertain bool, err error) {
	from := SMTPFrom
	if from == "" {
		from = SMTPAccount
	}
	message, err := BuildAttachmentMail(subject, receiver, content, from, files)
	if err != nil || SMTPServer == "" || SMTPPort <= 0 || SMTPPort > 65535 {
		return false, errors.New("invalid SMTP configuration or message")
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(SMTPServer, strconv.Itoa(SMTPPort)), 10*time.Second)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return false, err
	}
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		tlsConn := tls.Client(conn, smtpTLSConfig())
		if err := tlsConn.Handshake(); err != nil {
			return false, err
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, SMTPServer)
	if err != nil {
		return false, err
	}
	defer client.Close()
	if SMTPStartTLSEnabled && !SMTPSSLEnabled {
		if err := client.StartTLS(smtpTLSConfig()); err != nil {
			return false, err
		}
	}
	if shouldAuthenticateSMTP() {
		if err := client.Auth(getSMTPAuth()); err != nil {
			return false, err
		}
	}
	if err := client.Mail(from); err != nil {
		return false, err
	}
	if err := client.Rcpt(receiver); err != nil {
		return false, err
	}
	w, err := client.Data()
	if err != nil {
		return false, err
	}
	if _, err := w.Write(message); err != nil {
		return true, err
	}
	if err := w.Close(); err != nil {
		return true, err
	}
	_ = client.Quit()
	return false, nil
}
