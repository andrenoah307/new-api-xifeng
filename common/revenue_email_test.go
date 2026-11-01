package common

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueMailAcceptanceAndUncertainty(t *testing.T) {
	for _, scenario := range []string{"accepted-quit-disconnect", "lost-data-ack", "recipient-rejected"} {
		t.Run(scenario, func(t *testing.T) {
			withSMTPSettings(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			host, port, err := net.SplitHostPort(listener.Addr().String())
			require.NoError(t, err)
			SMTPServer = host
			SMTPPort, err = strconv.Atoi(port)
			require.NoError(t, err)
			SMTPFrom = "sender@example.com"
			SMTPAccount = ""
			SMTPToken = ""
			SMTPSSLEnabled = false
			SMTPStartTLSEnabled = false
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
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "MAIL"):
						_ = writeSMTPLine(rw, "250 OK")
					case strings.HasPrefix(line, "RCPT"):
						if scenario == "recipient-rejected" {
							_ = writeSMTPLine(rw, "550 rejected")
						} else {
							_ = writeSMTPLine(rw, "250 OK")
						}
					case strings.HasPrefix(line, "DATA"):
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
						if scenario == "lost-data-ack" {
							return
						}
						_ = writeSMTPLine(rw, "250 queued")
					case strings.HasPrefix(line, "QUIT"):
						return
					}
				}
			}()
			state, err := SendRevenueEmail(context.Background(), "Revenue", "finance@example.com", "<p>Profit: -5</p>")
			switch scenario {
			case "accepted-quit-disconnect":
				require.NoError(t, err)
				assert.Equal(t, "accepted", state)
			case "lost-data-ack":
				require.Error(t, err)
				assert.Equal(t, "unknown", state)
			case "recipient-rejected":
				require.Error(t, err)
				assert.Equal(t, "rejected", state)
			}
			<-done
		})
	}
}
