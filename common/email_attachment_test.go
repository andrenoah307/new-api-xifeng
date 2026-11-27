package common

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachmentSessionAuthenticatesOnceForMultipleMessages(t *testing.T) {
	withSMTPSettings(t)
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	SMTPServer, SMTPPort = server.host, server.port
	SMTPAccount, SMTPToken, SMTPFrom = "sender@example.test", "secret", "sender@example.test"
	SMTPSSLEnabled, SMTPStartTLSEnabled, SMTPForceAuthLogin = false, false, false
	session := &smtpMailSession{}
	defer session.Close()
	for _, receiver := range []string{"first@example.test", "second@example.test"} {
		uncertain, err := session.Send("Invoice", receiver, "Sent", []EmailAttachment{{Name: "123.png", Data: []byte{1}}})
		require.NoError(t, err)
		assert.False(t, uncertain)
		select {
		case message := <-server.messages:
			assert.Contains(t, message, "To: "+receiver)
		default:
			t.Fatal("missing message")
		}
		if receiver == "first@example.test" {
			select {
			case <-server.authCommands:
			default:
				t.Fatal("missing authentication")
			}
		} else {
			select {
			case <-server.authCommands:
				t.Fatal("connection authenticated twice")
			default:
			}
		}
	}
}

func TestAttachmentSMTPRetrySafety(t *testing.T) {
	for _, tc := range []struct {
		name, stage   string
		code, attempt int
		delay         time.Duration
	}{
		{"auth cooldown", "auth", 535, 1, 15 * time.Minute},
		{"last retry", "auth", 535, 2, 15 * time.Minute},
		{"exhausted", "auth", 535, 3, 0},
		{"temporary recipient", "recipient", 450, 2, 2 * time.Minute},
		{"permanent recipient", "recipient", 550, 1, 0},
		{"DATA not started", "data-command", 451, 1, time.Minute},
		{"DATA ambiguous", "data-ack", 451, 1, 0},
		{"DATA partial", "data-body", 421, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &SMTPDeliveryError{Stage: tc.stage, Err: &textproto.Error{Code: tc.code, Msg: "test"}}
			assert.Equal(t, tc.delay, SMTPRetryDelay(err, tc.attempt))
		})
	}
}

func TestAttachmentMailMIMEAndHeaderSafety(t *testing.T) {
	files := []EmailAttachment{{Name: "123.png", Data: []byte{0, 1, 2, 3}}, {Name: "123（2）.png", Data: []byte{4, 5, 6}}}
	encoded, err := BuildAttachmentMail("Invoice", "receiver@example.test", "<p>Sent</p>", "sender@example.test", files)
	require.NoError(t, err)
	message, err := mail.ReadMessage(bytes.NewReader(encoded))
	require.NoError(t, err)
	media, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	require.NoError(t, err)
	assert.Equal(t, "multipart/mixed", media)
	reader := multipart.NewReader(message.Body, params["boundary"])
	body, err := reader.NextPart()
	require.NoError(t, err)
	data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, body))
	require.NoError(t, err)
	assert.Equal(t, "<p>Sent</p>", string(data))
	for _, file := range files {
		part, err := reader.NextPart()
		require.NoError(t, err)
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		require.NoError(t, err)
		assert.Equal(t, file.Name, params["filename"])
		data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		require.NoError(t, err)
		assert.Equal(t, file.Data, data)
	}
	_, err = reader.NextPart()
	require.ErrorIs(t, err, io.EOF)
	_, err = BuildAttachmentMail("Invoice\r\nBcc: attacker@example.test", "receiver@example.test", "", "sender@example.test", files)
	require.Error(t, err)
	_, err = BuildAttachmentMail("Invoice", "a@example.test,b@example.test", "", "sender@example.test", files)
	require.Error(t, err)
	_, err = BuildAttachmentMail("Invoice", "a@example.test", "", "sender@example.test", []EmailAttachment{{Name: "x\r\nBcc: attacker", Data: []byte{1}}})
	require.Error(t, err)
}

func TestAttachmentSMTPAmbiguousDataAndFailedQuit(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted with missing QUIT reply", false: "missing DATA acknowledgment"}[accepted], func(t *testing.T) {
			withSMTPSettings(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
				_ = writeSMTPLine(rw, "220 test")
				for {
					line, err := rw.ReadString('\n')
					if err != nil {
						return
					}
					if strings.HasPrefix(line, "QUIT") {
						return
					}
					if strings.HasPrefix(line, "DATA") {
						_ = writeSMTPLine(rw, "354 send")
						for {
							line, err = rw.ReadString('\n')
							if err != nil {
								return
							}
							if line == ".\r\n" {
								break
							}
						}
						if !accepted {
							return
						}
					}
					_ = writeSMTPLine(rw, "250 OK")
				}
			}()
			SMTPServer = "127.0.0.1"
			SMTPPort = listener.Addr().(*net.TCPAddr).Port
			SMTPFrom = "sender@example.test"
			SMTPAccount = ""
			SMTPToken = ""
			SMTPSSLEnabled = false
			SMTPStartTLSEnabled = false
			SMTPForceAuthLogin = false
			uncertain, err := SendEmailAttachments("Invoice", "receiver@example.test", "Sent", []EmailAttachment{{Name: "123.png", Data: []byte{1}}})
			if accepted {
				require.NoError(t, err)
				assert.False(t, uncertain)
			} else {
				require.Error(t, err)
				assert.True(t, uncertain)
			}
			CloseEmailSession()
			<-done
		})
	}
}
