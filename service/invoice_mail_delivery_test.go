package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceMailUploadToSMTPAndCleanup(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.Ticket{}, &model.TicketMessage{}, &model.TicketInvoice{}, &model.TicketRefund{}, &model.InvoiceTaxCharge{}, &model.RefundOrderAllocation{}, &model.RefundInvoiceCorrection{}, &model.RefundInvoiceExport{}, &model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}))
	t.Cleanup(func() {
		for _, table := range []any{&model.Ticket{}, &model.TicketMessage{}, &model.TicketInvoice{}, &model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}} {
			require.NoError(t, model.DB.Where("1 = 1").Delete(table).Error)
		}
	})
	oldKind, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage = "local"
	setting.TicketAttachmentLocalPath = t.TempDir()
	t.Cleanup(func() { setting.TicketAttachmentStorage = oldKind; setting.TicketAttachmentLocalPath = oldPath })
	actor := model.User{Id: 930020, Username: "sender", AffCode: "sender", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}
	owner := model.User{Id: 930021, Username: "recipient", AffCode: "recipient", Email: "account@example.test", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	require.NoError(t, model.DB.Create(&owner).Error)
	order := model.TopUp{UserId: owner.Id, TradeNo: "invoice-mail-integration", Money: 100, Status: common.TopUpStatusSuccess}
	require.NoError(t, model.DB.Create(&order).Error)
	ticket := model.Ticket{UserId: owner.Id, Subject: "Invoice", Type: model.TicketTypeInvoice, Status: model.TicketStatusProcessing}
	require.NoError(t, model.DB.Create(&ticket).Error)
	invoice := model.TicketInvoice{TicketId: ticket.Id, UserId: owner.Id, Email: "invoice@example.test", InvoiceStatus: model.InvoiceStatusPending}
	require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
	require.NoError(t, model.DB.Create(&invoice).Error)
	var picture bytes.Buffer
	require.NoError(t, png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	filename := filepath.Join(t.TempDir(), "invoice.png")
	require.NoError(t, os.WriteFile(filename, picture.Bytes(), 0600))
	files := []InvoiceArchiveFile{{TicketID: ticket.Id, Name: fmt.Sprintf("%d.png", ticket.Id), Path: filename, Digest: fmt.Sprintf("%x", sha256.Sum256(picture.Bytes())), Size: int64(picture.Len())}}
	preview, err := CreateInvoiceMailBatch(context.Background(), actor.Id, files, nil)
	require.NoError(t, err)
	require.Len(t, preview.Items, 1)
	assert.Equal(t, "invoice@example.test", preview.Items[0].Recipient)
	assert.Equal(t, "ready", preview.Items[0].State)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	messages := make(chan string, 1)
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
		fmt.Fprint(rw, "220 test\r\n")
		_ = rw.Flush()
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "DATA") {
				fmt.Fprint(rw, "354 send\r\n")
				_ = rw.Flush()
				var message strings.Builder
				for {
					line, err = rw.ReadString('\n')
					if err != nil {
						return
					}
					if line == ".\r\n" {
						break
					}
					message.WriteString(line)
				}
				messages <- message.String()
			}
			if strings.HasPrefix(line, "QUIT") {
				fmt.Fprint(rw, "221 bye\r\n")
				_ = rw.Flush()
				return
			}
			fmt.Fprint(rw, "250 OK\r\n")
			_ = rw.Flush()
		}
	}()
	oldServer, oldPort, oldFrom, oldAccount, oldToken := common.SMTPServer, common.SMTPPort, common.SMTPFrom, common.SMTPAccount, common.SMTPToken
	oldSSL, oldStartTLS := common.SMTPSSLEnabled, common.SMTPStartTLSEnabled
	common.SMTPServer = "127.0.0.1"
	common.SMTPPort = listener.Addr().(*net.TCPAddr).Port
	common.SMTPFrom = "sender@example.test"
	common.SMTPAccount = ""
	common.SMTPToken = ""
	common.SMTPSSLEnabled = false
	common.SMTPStartTLSEnabled = false
	t.Cleanup(func() {
		common.SMTPServer = oldServer
		common.SMTPPort = oldPort
		common.SMTPFrom = oldFrom
		common.SMTPAccount = oldAccount
		common.SMTPToken = oldToken
		common.SMTPSSLEnabled = oldSSL
		common.SMTPStartTLSEnabled = oldStartTLS
	})
	require.NoError(t, SendInvoiceMailBatchItem(context.Background(), actor.Id, preview.ID, ticket.Id, ""))
	<-done
	select {
	case message := <-messages:
		assert.Contains(t, message, "To: invoice@example.test\r\n")
		assert.Contains(t, message, "Content-Type: image/png")
	default:
		t.Fatal("missing SMTP message")
	}
	preview, err = PreviewInvoiceMailBatch(actor.Id, preview.ID)
	require.NoError(t, err)
	assert.Equal(t, "sent", preview.Items[0].State)
	assert.Positive(t, preview.CleanedAt)
	require.NoError(t, model.DB.First(&invoice, invoice.Id).Error)
	assert.Equal(t, model.InvoiceStatusIssued, invoice.InvoiceStatus)
	require.NoError(t, model.DB.First(&ticket, ticket.Id).Error)
	assert.Equal(t, model.TicketStatusResolved, ticket.Status)
	var replies []model.TicketMessage
	require.NoError(t, model.DB.Where("ticket_id = ?", ticket.Id).Find(&replies).Error)
	require.Len(t, replies, 1)
	assert.Equal(t, model.DefaultInvoiceMailReply, replies[0].Content)
	require.NoError(t, filepath.WalkDir(setting.TicketAttachmentLocalPath, func(_ string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		assert.True(t, entry.IsDir(), "temporary image must be deleted")
		return nil
	}))
	// The same archive uploaded again is a sent receipt, not another SMTP call.
	again, err := CreateInvoiceMailBatch(context.Background(), actor.Id, files, nil)
	require.NoError(t, err)
	assert.Equal(t, "sent", again.Items[0].State)
	assert.Positive(t, again.CleanedAt)
	require.NoError(t, SendInvoiceMailBatchItem(context.Background(), actor.Id, again.ID, ticket.Id, ""))
}
