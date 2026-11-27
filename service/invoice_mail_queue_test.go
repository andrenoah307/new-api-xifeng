package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func invoiceQueueFixture(t *testing.T, mode string) (int, *InvoiceMailPreview, *miniredis.Miniredis, *atomic.Int32) {
	t.Helper()
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.Ticket{}, &model.TicketMessage{}, &model.TicketInvoice{}, &model.TicketRefund{}, &model.InvoiceTaxCharge{}, &model.RefundOrderAllocation{}, &model.RefundInvoiceCorrection{}, &model.RefundInvoiceExport{}, &model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}))
	t.Cleanup(func() {
		for _, table := range []any{&model.Ticket{}, &model.TicketMessage{}, &model.TicketInvoice{}, &model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}} {
			require.NoError(t, model.DB.Where("1 = 1").Delete(table).Error)
		}
	})
	oldKind, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = "local", t.TempDir()
	t.Cleanup(func() { setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = oldKind, oldPath })
	oldRedis, oldEnabled := common.RDB, common.RedisEnabled
	mr := miniredis.RunT(t)
	common.RDB, common.RedisEnabled = redis.NewClient(&redis.Options{Addr: mr.Addr()}), true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB, common.RedisEnabled = oldRedis, oldEnabled })
	oldServer, oldPort, oldFrom, oldAccount, oldToken := common.SMTPServer, common.SMTPPort, common.SMTPFrom, common.SMTPAccount, common.SMTPToken
	oldSSL, oldTLS, oldLogin := common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPForceAuthLogin
	t.Cleanup(func() {
		common.CloseEmailSession()
		common.SMTPServer, common.SMTPPort, common.SMTPFrom, common.SMTPAccount, common.SMTPToken = oldServer, oldPort, oldFrom, oldAccount, oldToken
		common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPForceAuthLogin = oldSSL, oldTLS, oldLogin
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	count := &atomic.Int32{}
	t.Cleanup(func() { common.CloseEmailSession(); _ = listener.Close(); <-done })
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
			fmt.Fprint(rw, "220 test\r\n")
			_ = rw.Flush()
			for {
				line, err := rw.ReadString('\n')
				if err != nil {
					break
				}
				response := "250 OK\r\n"
				switch {
				case strings.HasPrefix(line, "EHLO"):
					response = "250-test\r\n250 AUTH PLAIN\r\n"
				case strings.HasPrefix(line, "AUTH"):
					response = "235 authenticated\r\n"
					if mode == "535" {
						response = "535 login frequency limited\r\n"
						count.Add(1)
					}
				case strings.HasPrefix(line, "QUIT"):
					_ = conn.Close()
				case strings.HasPrefix(line, "DATA"):
					fmt.Fprint(rw, "354 send\r\n")
					_ = rw.Flush()
					for {
						line, err = rw.ReadString('\n')
						if err != nil || line == ".\r\n" {
							break
						}
					}
					count.Add(1)
					if mode == "unknown" {
						_ = conn.Close()
					}
				}
				fmt.Fprint(rw, response)
				_ = rw.Flush()
			}
			_ = conn.Close()
		}
	}()
	common.SMTPServer, common.SMTPPort = "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
	common.SMTPAccount, common.SMTPFrom, common.SMTPToken = "sender@example.test", "sender@example.test", "test"
	common.SMTPSSLEnabled, common.SMTPStartTLSEnabled, common.SMTPForceAuthLogin = false, false, false
	actor := model.User{Id: 940020, Username: "queue-finance", AffCode: "queue-finance", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}
	owner := model.User{Id: 940021, Username: "queue-recipient", AffCode: "queue-recipient", Email: "recipient@example.test", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	require.NoError(t, model.DB.Create(&owner).Error)
	var files []InvoiceArchiveFile
	for i := 0; i < 2; i++ {
		order := model.TopUp{UserId: owner.Id, TradeNo: fmt.Sprintf("queue-%d", i), Money: 100, Status: common.TopUpStatusSuccess}
		require.NoError(t, model.DB.Create(&order).Error)
		ticket := model.Ticket{UserId: owner.Id, Subject: "Invoice", Type: model.TicketTypeInvoice, Status: model.TicketStatusProcessing}
		require.NoError(t, model.DB.Create(&ticket).Error)
		invoice := model.TicketInvoice{TicketId: ticket.Id, UserId: owner.Id, InvoiceStatus: model.InvoiceStatusPending}
		require.NoError(t, invoice.SetTopUpOrderIDs([]int{order.Id}))
		require.NoError(t, model.DB.Create(&invoice).Error)
		data := []byte("validated invoice attachment")
		filename := filepath.Join(t.TempDir(), "invoice.png")
		require.NoError(t, os.WriteFile(filename, data, 0600))
		files = append(files, InvoiceArchiveFile{TicketID: ticket.Id, Name: fmt.Sprintf("%d.png", ticket.Id), Path: filename, Digest: fmt.Sprintf("%x", sha256.Sum256(data)), Size: int64(len(data))})
	}
	preview, err := CreateInvoiceMailBatch(context.Background(), actor.Id, files, nil)
	require.NoError(t, err)
	return actor.Id, preview, mr, count
}

func TestInvoiceMailQueueRecoveryPacingAndIdempotency(t *testing.T) {
	actor, batch, mr, count := invoiceQueueFixture(t, "success")
	ctx := context.Background()
	preview, err := EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	require.Len(t, preview.Queue.Items, 2)
	assert.Zero(t, count.Load(), "enqueue must not send")
	var stored model.InvoiceMailBatch
	require.NoError(t, model.DB.First(&stored, "id = ?", batch.ID).Error)
	originalPayload := stored.QueuePayload
	mr.FlushAll()
	require.NoError(t, reconcileInvoiceMailQueue(ctx))
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 1, count.Load())
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 1, count.Load(), "cooldown prevents next send")
	// Simulate restart after SMTP/receipt commit but before queue progress commit.
	require.NoError(t, model.DB.Model(&stored).Updates(map[string]any{"queue_payload": originalPayload, "queue_status": "running", "queue_next_at": 0}).Error)
	mr.FlushAll()
	require.NoError(t, reconcileInvoiceMailQueue(ctx))
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 1, count.Load(), "durable receipt prevents duplicate mail")
	mr.FastForward(10 * time.Second)
	require.NoError(t, model.DB.Model(&stored).Update("queue_next_at", 0).Error)
	require.NoError(t, reconcileInvoiceMailQueue(ctx))
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 2, count.Load())
	preview, err = PreviewInvoiceMailBatch(actor, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", preview.Queue.Status)
	assert.Equal(t, 2, preview.Queue.Position)
	assert.Positive(t, preview.CleanedAt)
	var replies int64
	require.NoError(t, model.DB.Model(&model.TicketMessage{}).Count(&replies).Error)
	assert.EqualValues(t, 2, replies)
}

func TestInvoiceMailQueueAuthCooldownAndBoundedRetries(t *testing.T) {
	actor, batch, mr, count := invoiceQueueFixture(t, "535")
	ctx := context.Background()
	_, err := EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	for attempt := 1; attempt <= 3; attempt++ {
		require.NoError(t, runInvoiceMailQueueOnce(ctx))
		preview, err := PreviewInvoiceMailBatch(actor, batch.ID)
		require.NoError(t, err)
		assert.Equal(t, attempt, preview.Queue.Attempts)
		assert.Equal(t, "failed", preview.Items[0].State)
		assert.EqualValues(t, attempt, count.Load())
		assert.Equal(t, 15*time.Minute, mr.TTL(common.EmailSMTPKey()))
		if attempt == 3 {
			assert.Equal(t, "paused", preview.Queue.Status)
			break
		}
		assert.Equal(t, "queued", preview.Queue.Status)
		assert.GreaterOrEqual(t, preview.Queue.NextAt, common.GetTimestamp()+899)
		require.NoError(t, runInvoiceMailQueueOnce(ctx))
		assert.EqualValues(t, attempt, count.Load())
		mr.FastForward(15 * time.Minute)
		require.NoError(t, model.DB.Model(&model.InvoiceMailBatch{}).Where("id = ?", batch.ID).Update("queue_next_at", 0).Error)
		require.NoError(t, reconcileInvoiceMailQueue(ctx))
	}
	var replies int64
	require.NoError(t, model.DB.Model(&model.TicketMessage{}).Count(&replies).Error)
	assert.Zero(t, replies, "failed SMTP must not resolve or reply")
}

func TestInvoiceMailQueueUnknownNeverRetriesAutomatically(t *testing.T) {
	actor, batch, mr, count := invoiceQueueFixture(t, "unknown")
	ctx := context.Background()
	_, err := EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	preview, err := PreviewInvoiceMailBatch(actor, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, "unknown", preview.Items[0].State)
	assert.Equal(t, "paused", preview.Queue.Status)
	mr.FastForward(time.Hour)
	require.NoError(t, reconcileInvoiceMailQueue(ctx))
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 1, count.Load())
}

func TestInvoiceMailQueuePauseAndRevokePermission(t *testing.T) {
	actor, batch, mr, count := invoiceQueueFixture(t, "success")
	ctx := context.Background()
	_, err := EnqueueInvoiceMailBatch(ctx, actor+1, batch.ID, 0, "")
	require.Error(t, err)
	common.RedisEnabled = false
	_, err = EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.Error(t, err)
	common.RedisEnabled = true
	_, err = EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	_, err = PauseInvoiceMailBatch(ctx, actor, batch.ID)
	require.NoError(t, err)
	_ = runInvoiceMailQueueOnce(ctx)
	assert.Zero(t, count.Load())
	_, err = EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actor).Update("role", common.RoleAdminUser).Error)
	mr.FastForward(time.Minute)
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.Zero(t, count.Load(), "worker rechecks current stored role")
}
