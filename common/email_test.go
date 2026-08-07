package common

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rfc5321MaxLineOctets 是 RFC 5321 §4.5.3.1.6 规定的文本行上限（不含 CRLF）。
// 超出该长度时 MTA 会以 "500 Line too long" 拒收整封邮件。
const rfc5321MaxLineOctets = 998

// requireLinesWithinRFC5321 断言报文每一行都在 998 octet 以内。
// 这是本文件保护的核心不变量：任何组装路径都不得产出超长行。
func requireLinesWithinRFC5321(t *testing.T, mail string) {
	t.Helper()
	for i, line := range strings.Split(mail, "\r\n") {
		require.LessOrEqualf(t, len(line), rfc5321MaxLineOctets,
			"第 %d 行长度 %d octet 超出 RFC 5321 上限", i+1, len(line))
	}
}

// decodeMailBody 拆出报文正文并做 base64 解码，用于验证编码可逆。
func decodeMailBody(t *testing.T, mail string) string {
	t.Helper()
	parts := strings.SplitN(mail, "\r\n\r\n", 2)
	require.Len(t, parts, 2, "报文缺少 header/body 分隔的空行")
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(parts[1], "\r\n", ""))
	require.NoError(t, err, "正文不是合法 base64")
	return string(decoded)
}

// requireDeliveredBody 断言 fake SMTP 服务端实收报文行长合规，且正文解码后与预期一致。
func requireDeliveredBody(t *testing.T, mail, wantBody string) {
	t.Helper()
	requireLinesWithinRFC5321(t, mail)
	require.Equal(t, wantBody, decodeMailBody(t, mail))
}

// headerValue 取出指定 header 的值，并按 RFC 5322 §2.2.3 展开折叠续行。
func headerValue(t *testing.T, mail, name string) string {
	t.Helper()
	lines := strings.Split(strings.SplitN(mail, "\r\n\r\n", 2)[0], "\r\n")
	prefix := name + ": "
	for i, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimPrefix(line, prefix)
		for _, next := range lines[i+1:] {
			if next == "" || (next[0] != ' ' && next[0] != '\t') {
				break
			}
			value += " " + strings.TrimLeft(next, " \t")
		}
		return value
	}
	require.Failf(t, "header 未找到", "%s", name)
	return ""
}

type fakeSMTPServer struct {
	listener          net.Listener
	host              string
	port              int
	cert              tls.Certificate
	advertiseSTARTTLS bool
	authMechanisms    []string
	messages          chan string
	authCommands      chan string
	startTLSCommands  chan string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	return newFakeSMTPServerWithSTARTTLSAdvertisement(t, true)
}

func newFakeSMTPServerWithSTARTTLSAdvertisement(t *testing.T, advertiseSTARTTLS bool) *fakeSMTPServer {
	t.Helper()

	cert, err := newTestTLSCertificate()
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener:          listener,
		host:              host,
		port:              port,
		cert:              cert,
		advertiseSTARTTLS: advertiseSTARTTLS,
		authMechanisms:    []string{"PLAIN", "LOGIN"},
		messages:          make(chan string, 1),
		authCommands:      make(chan string, 1),
		startTLSCommands:  make(chan string, 1),
	}
	go server.serve()
	return server
}

func newFakeImplicitTLSSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()

	cert, err := newTestTLSCertificate()
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener:          tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}}),
		host:              host,
		port:              port,
		cert:              cert,
		advertiseSTARTTLS: false,
		authMechanisms:    []string{"PLAIN", "LOGIN"},
		messages:          make(chan string, 1),
		authCommands:      make(chan string, 1),
		startTLSCommands:  make(chan string, 1),
	}
	go server.serve()
	return server
}

func (s *fakeSMTPServer) close() {
	_ = s.listener.Close()
}

func (s *fakeSMTPServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeSMTPLine(rw, "220 fake.smtp.local ESMTP"); err != nil {
		return
	}

	encrypted := false
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimRight(line, "\r\n")
		upperCommand := strings.ToUpper(command)

		switch {
		case strings.HasPrefix(upperCommand, "EHLO"):
			if err := writeSMTPLine(rw, "250-fake.smtp.local"); err != nil {
				return
			}
			if !encrypted && s.advertiseSTARTTLS {
				if err := writeSMTPLine(rw, "250-STARTTLS"); err != nil {
					return
				}
			}
			if len(s.authMechanisms) > 0 {
				if err := writeSMTPLine(rw, "250 AUTH "+strings.Join(s.authMechanisms, " ")); err != nil {
					return
				}
			} else if err := writeSMTPLine(rw, "250 8BITMIME"); err != nil {
				return
			}
		case upperCommand == "STARTTLS":
			if encrypted || !s.advertiseSTARTTLS {
				if err := writeSMTPLine(rw, "502 5.5.1 STARTTLS not supported"); err != nil {
					return
				}
				continue
			}
			select {
			case s.startTLSCommands <- command:
			default:
			}
			if err := writeSMTPLine(rw, "220 2.0.0 Ready to start TLS"); err != nil {
				return
			}
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
			encrypted = true
		case strings.HasPrefix(upperCommand, "AUTH"):
			select {
			case s.authCommands <- command:
			default:
			}
			if err := writeSMTPLine(rw, "235 2.7.0 Authentication successful"); err != nil {
				return
			}
		case strings.HasPrefix(upperCommand, "MAIL FROM:"):
			if err := writeSMTPLine(rw, "250 2.1.0 Sender OK"); err != nil {
				return
			}
		case strings.HasPrefix(upperCommand, "RCPT TO:"):
			if err := writeSMTPLine(rw, "250 2.1.5 Recipient OK"); err != nil {
				return
			}
		case upperCommand == "DATA":
			if err := writeSMTPLine(rw, "354 End data with <CR><LF>.<CR><LF>"); err != nil {
				return
			}
			var data strings.Builder
			for {
				dataLine, err := rw.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				data.WriteString(dataLine)
			}
			s.messages <- data.String()
			if err := writeSMTPLine(rw, "250 2.0.0 Queued"); err != nil {
				return
			}
		case upperCommand == "QUIT":
			_ = writeSMTPLine(rw, "221 2.0.0 Bye")
			return
		default:
			if err := writeSMTPLine(rw, "502 5.5.1 Command not implemented"); err != nil {
				return
			}
		}
	}
}

func writeSMTPLine(rw *bufio.ReadWriter, line string) error {
	_, err := rw.WriteString(line + "\r\n")
	if err != nil {
		return err
	}
	return rw.Flush()
}

func newTestTLSCertificate() (tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "aixinexchange01.aixin-chip.com",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"aixinexchange01", "aixinexchange01.aixin-chip.com"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func withSMTPSettings(t *testing.T) {
	t.Helper()
	originalSMTPServer := SMTPServer
	originalSMTPPort := SMTPPort
	originalSMTPSSLEnabled := SMTPSSLEnabled
	originalSMTPStartTLSEnabled := SMTPStartTLSEnabled
	originalSMTPInsecureSkipVerify := SMTPInsecureSkipVerify
	originalSMTPForceAuthLogin := SMTPForceAuthLogin
	originalSMTPAccount := SMTPAccount
	originalSMTPFrom := SMTPFrom
	originalSMTPToken := SMTPToken
	originalSystemName := SystemName

	t.Cleanup(func() {
		SMTPServer = originalSMTPServer
		SMTPPort = originalSMTPPort
		SMTPSSLEnabled = originalSMTPSSLEnabled
		SMTPStartTLSEnabled = originalSMTPStartTLSEnabled
		SMTPInsecureSkipVerify = originalSMTPInsecureSkipVerify
		SMTPForceAuthLogin = originalSMTPForceAuthLogin
		SMTPAccount = originalSMTPAccount
		SMTPFrom = originalSMTPFrom
		SMTPToken = originalSMTPToken
		SystemName = originalSystemName
	})
}

func TestSendEmailUsesExplicitStartTLSWithInsecureCertificate(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case message := <-server.messages:
		requireDeliveredBody(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailExplicitStartTLSRequiresServerSupport(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.Error(t, err)
	require.Contains(t, err.Error(), "STARTTLS")
}

func TestSendEmailDoesNotAutoUpgradeWhenStartTLSDisabled(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, true)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.startTLSCommands:
		t.Fatalf("unexpected SMTP STARTTLS command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		requireDeliveredBody(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSMTPPlainAuthRejectsRemotePlaintextConnection(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = "smtp.example.com"
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	client, err := smtp.NewClient(conn, SMTPServer)
	require.NoError(t, err)

	err = client.Auth(getSMTPAuth())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unencrypted connection")

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}
}

func TestNewSMTPClientHonorsExplicitStartTLSWhenPortIs465(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = 465
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true

	client, err := newSMTPClient(fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	defer client.Close()

	select {
	case command := <-server.startTLSCommands:
		require.Equal(t, "STARTTLS", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP STARTTLS")
	}
}

func TestNewSMTPClientKeepsImplicitTLSForLegacyPort465(t *testing.T) {
	server := newFakeImplicitTLSSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = 465
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = true

	client, err := newSMTPClient(fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	defer client.Close()
}

func TestSendEmailSkipsAuthWhenCredentialsAreEmpty(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = ""
	SMTPFrom = "sender@example.com"
	SMTPToken = ""
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		requireDeliveredBody(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailSkipsAuthWhenCredentialsAreIncomplete(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = ""
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		requireDeliveredBody(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailUsesNTLMWhenServerOnlySupportsNTLM(t *testing.T) {
	server := newFakeSMTPServer(t)
	server.authMechanisms = []string{"NTLM"}
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "no-reply"
	SMTPFrom = "no-reply@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		require.True(t, strings.HasPrefix(command, "AUTH NTLM "), "unexpected auth command: %s", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP AUTH")
	}
}

func TestSendEmailUsesNTLMForMicrosoftAccountWhenServerOnlySupportsNTLM(t *testing.T) {
	server := newFakeSMTPServer(t)
	server.authMechanisms = []string{"NTLM"}
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "no-reply@contoso.onmicrosoft.com"
	SMTPFrom = "no-reply@contoso.onmicrosoft.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		require.True(t, strings.HasPrefix(command, "AUTH NTLM "), "unexpected auth command: %s", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP AUTH")
	}
}

func TestSendEmailExplicitStartTLSRejectsUntrustedCertificateByDefault(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.Error(t, err)
	require.Contains(t, fmt.Sprint(err), "certificate")
}

// TestBuildMailMessageKeepsEveryLineWithinRFC5321 是本次修复的核心回归锚点：
// 该模板曾因 RenderInfoTableHTML 产出 2000+ octet 的零换行单行，被 MTA 以
// "500 Line too long" 全站拒收。传输层必须对任意正文都保证行长合规且编码可逆。
func TestBuildMailMessageKeepsEveryLineWithinRFC5321(t *testing.T) {
	withSMTPSettings(t)
	SMTPFrom = "sender@example.com"
	SystemName = "New API"

	cases := []struct {
		name    string
		content string
	}{
		{name: "空正文", content: ""},
		{name: "纯 ASCII", content: "<p>123456</p>"},
		{name: "中文正文", content: "<p>验证码：123456，五分钟内有效。</p>"},
		{name: "超长单行", content: "<td>" + strings.Repeat("x", 4000) + "</td>"},
		{name: "超长中文单行", content: strings.Repeat("工单编号", 500)},
		{name: "恰好 997 octet", content: strings.Repeat("a", 997)},
		{name: "恰好 998 octet", content: strings.Repeat("a", 998)},
		{name: "恰好 999 octet", content: strings.Repeat("a", 999)},
		{name: "含 CRLF 与裸 LF", content: "line1\r\nline2\nline3"},
		{name: "行首为点", content: ".hidden\r\n.\r\n..dot"},
		{name: "多字节跨 76 列边界", content: strings.Repeat("中", 200)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mail, err := buildMailMessage("Verification", "receiver@example.com", tc.content)
			require.NoError(t, err)

			message := string(mail)
			requireLinesWithinRFC5321(t, message)
			assert.Contains(t, message, "MIME-Version: 1.0\r\n")
			assert.Contains(t, message, "Content-Transfer-Encoding: base64\r\n")
			require.Equal(t, tc.content, decodeMailBody(t, message), "base64 编码必须可逆")

			// base64 字符集恒为 A-Za-z0-9+/=，不会产出行首单独的点，
			// 因此与 textproto.DotWriter 的 dot-stuffing 不存在双重转义。
			for _, line := range strings.Split(strings.SplitN(message, "\r\n\r\n", 2)[1], "\r\n") {
				assert.False(t, strings.HasPrefix(line, "."), "base64 正文不应出现行首点：%q", line)
			}
		})
	}
}

func TestBuildMailMessageFoldsBase64BodyAt76Columns(t *testing.T) {
	withSMTPSettings(t)
	SMTPFrom = "sender@example.com"
	SystemName = "New API"

	mail, err := buildMailMessage("Verification", "receiver@example.com", strings.Repeat("a", 1000))
	require.NoError(t, err)

	body := strings.SplitN(string(mail), "\r\n\r\n", 2)[1]
	lines := strings.Split(strings.TrimSuffix(body, "\r\n"), "\r\n")
	require.NotEmpty(t, lines)
	for _, line := range lines[:len(lines)-1] {
		assert.Len(t, line, 76, "除末行外每行都应折叠到 76 列")
	}
	assert.LessOrEqual(t, len(lines[len(lines)-1]), 76)
}

// TestBuildMailMessageEncodesSubjectHeader 覆盖 S1：ticket.Subject 是 varchar(255)，
// 255 个中文经 base64 encoded-word 后约 1033 octet，手写 encoded-word 会再次触发 500。
func TestBuildMailMessageEncodesSubjectHeader(t *testing.T) {
	withSMTPSettings(t)
	SMTPFrom = "sender@example.com"
	SystemName = "New API"

	decoder := mime.WordDecoder{}
	cases := []struct {
		name         string
		subject      string
		wantEncoding bool
	}{
		{name: "纯 ASCII 保持明文", subject: "Verification", wantEncoding: false},
		{name: "中文短主题", subject: "工单已回复", wantEncoding: true},
		{name: "255 个中文主题", subject: strings.Repeat("工", 255), wantEncoding: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mail, err := buildMailMessage(tc.subject, "receiver@example.com", "<p>hi</p>")
			require.NoError(t, err)

			message := string(mail)
			requireLinesWithinRFC5321(t, message)

			value := headerValue(t, message, "Subject")
			assert.Equal(t, tc.wantEncoding, strings.Contains(value, "=?UTF-8?"))
			decoded, err := decoder.DecodeHeader(value)
			require.NoError(t, err)
			require.Equal(t, tc.subject, decoded, "主题必须能被 RFC 2047 解码还原")
		})
	}
}

// TestBuildMailMessageEncodesNonASCIIFromDisplayName 覆盖 S2：生产 SystemName 为「米醋API 」，
// 直接塞进 From header 违反 RFC 5322 的 ASCII-only 要求。
func TestBuildMailMessageEncodesNonASCIIFromDisplayName(t *testing.T) {
	withSMTPSettings(t)
	SMTPFrom = "sender@example.com"
	SystemName = "米醋API "

	mail, err := buildMailMessage("Verification", "receiver@example.com", "<p>hi</p>")
	require.NoError(t, err)

	value := headerValue(t, string(mail), "From")
	for i := 0; i < len(value); i++ {
		require.Lessf(t, value[i], byte(0x80), "From header 第 %d 字节非 ASCII: %q", i, value)
	}
	require.True(t, strings.HasSuffix(value, " <sender@example.com>"), "From header 缺少地址部分: %q", value)

	decoded, err := (&mime.WordDecoder{}).DecodeHeader(strings.TrimSuffix(value, " <sender@example.com>"))
	require.NoError(t, err)
	require.Equal(t, SystemName, decoded)
}

// TestBuildMailMessageWithRenderedTicketTemplate 是端到端锚点：直接喂入真实渲染器的输出，
// 复现生产上 8 行信息表 + 500 rune 预览块的报文形态。
func TestBuildMailMessageWithRenderedTicketTemplate(t *testing.T) {
	withSMTPSettings(t)
	SMTPFrom = "sender@example.com"
	SystemName = "米醋API "

	rows := []EmailTemplateRow{
		{Label: "工单编号", Value: "#123456"},
		{Label: "主题", Value: EscapeAndBreak(strings.Repeat("接口调用异常", 20))},
		{Label: "类型", Value: "技术支持"},
		{Label: "优先级", Value: "紧急"},
		{Label: "当前状态", Value: "处理中"},
		{Label: "提交用户", Value: "user@example.com"},
		{Label: "创建时间", Value: "2026-08-07 12:00:00"},
		{Label: "最新回复", Value: EscapeAndBreak("已定位到上游渠道超时")},
	}
	content := RenderInfoTableHTML(rows) + RenderPreviewBlockHTML("内容预览", EscapeAndBreak(strings.Repeat("排查详情", 125)))
	require.Greater(t, len(content), rfc5321MaxLineOctets, "夹具必须能复现超长单行，否则回归失效")

	mail, err := buildMailMessage(strings.Repeat("工单主题", 60), "receiver@example.com", content)
	require.NoError(t, err)

	message := string(mail)
	requireLinesWithinRFC5321(t, message)
	require.Equal(t, content, decodeMailBody(t, message))
}
