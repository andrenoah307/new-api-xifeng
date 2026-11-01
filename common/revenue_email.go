package common

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SendRevenueEmail distinguishes acknowledged DATA from uncertain delivery.
// Existing ticket mail behavior is deliberately left unchanged.
func SendRevenueEmail(ctx context.Context, subject, receiver, body string) (state string, err error) {
	state = "retryable"
	if strings.ContainsAny(subject, "\r\n") || strings.ContainsAny(receiver, "\r\n;") {
		return "rejected", errors.New("invalid mail headers")
	}
	address, e := mail.ParseAddress(receiver)
	if e != nil || address.Address != receiver {
		return "rejected", errors.New("invalid recipient")
	}
	from := SMTPFrom
	if from == "" {
		from = SMTPAccount
	}
	if SMTPServer == "" || from == "" {
		return "rejected", errors.New("SMTP configuration is incomplete")
	}
	message, e := buildMailMessageWithFrom(subject, receiver, body, from)
	if e != nil {
		return "rejected", e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, e := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", SMTPServer, SMTPPort))
	if e != nil {
		return state, e
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if e = conn.SetDeadline(deadline); e != nil {
		return state, e
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
		secure := tls.Client(conn, smtpTLSConfig())
		if e = secure.HandshakeContext(ctx); e != nil {
			return state, e
		}
		conn = secure
	}
	client, e := smtp.NewClient(conn, SMTPServer)
	if e != nil {
		return state, e
	}
	defer client.Close()
	if SMTPStartTLSEnabled && !SMTPSSLEnabled {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return "rejected", errors.New("SMTP server does not support STARTTLS")
		}
		if e = client.StartTLS(smtpTLSConfig()); e != nil {
			return state, e
		}
	}
	if shouldAuthenticateSMTP() {
		if e = client.Auth(getSMTPAuth()); e != nil {
			return "rejected", e
		}
	}
	if e = client.Mail(from); e != nil {
		return state, e
	}
	if e = client.Rcpt(receiver); e != nil {
		return "rejected", e
	}
	w, e := client.Data()
	if e != nil {
		return state, e
	}
	// Once DATA can be accepted, a lost acknowledgement is not safe to replay.
	if _, e = w.Write(message); e != nil {
		return "unknown", e
	}
	if e = w.Close(); e != nil {
		return "unknown", e
	}
	_ = client.Quit()
	return "accepted", nil
}
