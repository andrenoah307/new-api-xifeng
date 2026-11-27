package common

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EmailAttachment struct {
	Name string
	Data []byte
}

type SMTPDeliveryError struct {
	Stage string
	Err   error
}

func (e *SMTPDeliveryError) Error() string { return "SMTP " + e.Stage + ": " + e.Err.Error() }
func (e *SMTPDeliveryError) Unwrap() error { return e.Err }

// Retry only failures known to occur before DATA. Authentication 535 can mean
// throttling or invalid credentials, so callers must bound retries and cool down.
func SMTPRetryDelay(err error, attempt int) time.Duration {
	if attempt < 1 || attempt >= 3 {
		return 0
	}
	var failure *SMTPDeliveryError
	if !errors.As(err, &failure) || failure.Stage == "data-body" || failure.Stage == "data-ack" {
		return 0
	}
	if errors.Is(err, ErrEmailQueueBusy) && failure.Stage == "message" {
		return time.Minute
	}
	var response *textproto.Error
	if errors.As(err, &response) {
		if response.Code == 535 && failure.Stage == "auth" {
			return 15 * time.Minute
		}
		if response.Code >= 400 && response.Code < 500 {
			return time.Duration(attempt) * time.Minute
		}
		return 0
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return time.Duration(attempt) * time.Minute
	}
	return 0
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
	if len(files) == 0 {
		return false, errors.New("invalid attachment message")
	}
	return sendQueuedSMTP(context.Background(), subject, receiver, content, files)
}

// smtpMailSession serializes messages on one authenticated connection.
// Idle sessions close automatically; errors discard the connection without
// retrying DATA, whose acknowledgement may have been lost.
type smtpMailSession struct {
	mu        sync.Mutex
	client    *smtp.Client
	conn      net.Conn
	config    [32]byte
	idle      *time.Timer
	idleUntil time.Time
}

func (s *smtpMailSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.close()
}

func (s *smtpMailSession) close() {
	if s.idle != nil {
		s.idle.Stop()
	}
	if s.client != nil {
		_ = s.client.Close()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.client, s.conn = nil, nil
}

func (s *smtpMailSession) Send(subject, receiver, content string, files []EmailAttachment) (uncertain bool, err error) {
	return s.send(context.Background(), subject, receiver, content, files, nil)
}

func (s *smtpMailSession) send(ctx context.Context, subject, receiver, content string, files []EmailAttachment, before func() error) (uncertain bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idle != nil {
		s.idle.Stop()
	}
	stage := "message"
	defer func() {
		if err != nil {
			s.close()
			err = &SMTPDeliveryError{Stage: stage, Err: err}
		} else {
			s.idleUntil = time.Now().Add(90 * time.Second)
			s.idle = time.AfterFunc(90*time.Second, func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				if !time.Now().Before(s.idleUntil) {
					s.close()
				}
			})
		}
	}()
	from := SMTPFrom
	if from == "" {
		from = SMTPAccount
	}
	var message []byte
	if len(files) > 0 {
		message, err = BuildAttachmentMail(subject, receiver, content, from, files)
	} else {
		if err = ValidateEmailMessage(subject, receiver, content); err == nil {
			message, err = buildMailMessageWithFrom(subject, receiver, content, from)
		}
	}
	if err != nil || SMTPServer == "" || SMTPPort <= 0 || SMTPPort > 65535 {
		return false, errors.New("invalid SMTP configuration or message")
	}
	config := sha256.Sum256([]byte(fmt.Sprintf("%q:%d:%q:%q:%q:%t:%t:%t:%t", SMTPServer, SMTPPort, SMTPAccount, SMTPToken, from, SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPForceAuthLogin, SMTPInsecureSkipVerify)))
	if config != s.config {
		s.close()
	}
	if before != nil {
		if err := before(); err != nil {
			return false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var stop func() bool
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	if s.client == nil {
		stage = "connect"
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(SMTPServer, strconv.Itoa(SMTPPort)))
		if err != nil {
			return false, err
		}
		s.conn = conn
		rawConn := conn
		stop = context.AfterFunc(ctx, func() { _ = rawConn.Close() })
		if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return false, err
		}
		if SMTPSSLEnabled || (SMTPPort == 465 && !SMTPStartTLSEnabled) {
			stage = "tls"
			tlsConn := tls.Client(conn, smtpTLSConfig())
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				return false, err
			}
			conn = tlsConn
		}
		stage = "greeting"
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			return false, err
		}
		s.client = client
		if SMTPStartTLSEnabled && !SMTPSSLEnabled {
			stage = "tls"
			if err := client.StartTLS(smtpTLSConfig()); err != nil {
				return false, err
			}
		}
		if shouldAuthenticateSMTP() {
			stage = "auth"
			if err := client.Auth(getSMTPAuth()); err != nil {
				return false, err
			}
		}
		s.config = config
	} else {
		stage = "mail"
		conn := s.conn
		stop = context.AfterFunc(ctx, func() { _ = conn.Close() })
		if err := s.conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return false, err
		}
	}
	client := s.client
	stage = "mail"
	if err := client.Mail(from); err != nil {
		return false, err
	}
	stage = "recipient"
	for _, recipient := range strings.Split(receiver, ";") {
		if err := client.Rcpt(recipient); err != nil {
			return false, err
		}
	}
	stage = "data-command"
	w, err := client.Data()
	if err != nil {
		return false, err
	}
	stage = "data-body"
	if _, err := w.Write(message); err != nil {
		return true, err
	}
	stage = "data-ack"
	if err := w.Close(); err != nil {
		return true, err
	}
	return false, nil
}
