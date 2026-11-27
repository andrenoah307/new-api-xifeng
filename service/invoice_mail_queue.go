package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvoiceMailQueueItem struct {
	TicketID int    `json:"ticket_id"`
	Review   string `json:"review,omitempty"`
}

// The database is authoritative; Redis only schedules and coordinates workers.
// No recipients, attachments or SMTP credentials are placed in Redis.
type InvoiceMailQueue struct {
	ActorID    int                    `json:"actor_id"`
	Generation string                 `json:"generation"`
	Items      []InvoiceMailQueueItem `json:"items"`
	Position   int                    `json:"position"`
	Attempts   int                    `json:"attempts"`
	Reason     string                 `json:"reason,omitempty"`
	Status     string                 `json:"status"`
	NextAt     int64                  `json:"next_at"`
}

func invoiceMailQueueKey() string {
	database := os.Getenv("SQL_DSN")
	if database == "" {
		database = common.SQLitePath
	}
	return fmt.Sprintf("invoice-mail:due:%x", sha256.Sum256([]byte(database)))
}

func EnqueueInvoiceMailBatch(ctx context.Context, actorID int, id string, ticketID int, review string) (*InvoiceMailPreview, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, model.ErrInvoiceMail
	}
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	preview, err := PreviewInvoiceMailBatch(actorID, id)
	if err != nil {
		return nil, err
	}
	if review != "" && review != "not_delivered" {
		return nil, model.ErrInvoiceMail
	}
	if preview.Queue != nil && (preview.Queue.Status == "queued" || preview.Queue.Status == "running") {
		if ticketID == 0 {
			return preview, nil
		}
		for _, item := range preview.Queue.Items {
			if item.TicketID == ticketID && item.Review == review {
				return preview, nil
			}
		}
		return nil, model.ErrInvoiceMail
	}
	queue := InvoiceMailQueue{ActorID: actorID, Generation: uuid.NewString()}
	for _, item := range preview.Items {
		if ticketID != 0 && item.TicketID != ticketID {
			continue
		}
		if (review == "" && (item.State == "ready" || item.State == "failed")) || (review == "not_delivered" && item.State == "unknown") {
			queue.Items = append(queue.Items, InvoiceMailQueueItem{TicketID: item.TicketID, Review: review})
		}
	}
	if len(queue.Items) == 0 {
		return nil, model.ErrInvoiceMail
	}
	encoded, err := common.Marshal(queue)
	if err != nil {
		return nil, err
	}
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		batch, err := model.LockInvoiceMailBatch(tx, id)
		if err != nil {
			return err
		}
		if batch.CancelledAt != 0 || batch.CleanedAt != 0 || batch.ExpiresAt <= common.GetTimestamp() {
			return model.ErrInvoiceMail
		}
		if batch.QueueStatus == "queued" || batch.QueueStatus == "running" {
			return model.ErrInvoiceMail
		}
		return tx.Model(batch).Updates(map[string]any{"queue_status": "queued", "queue_next_at": common.GetTimestamp(), "queue_payload": string(encoded)}).Error
	})
	if err != nil {
		return nil, err
	}
	// A cache outage after commit does not lose the accepted job. Periodic DB
	// reconciliation repopulates due work after Redis loss or process restart.
	if err := common.RDB.ZAdd(ctx, invoiceMailQueueKey(), &redis.Z{Score: float64(common.GetTimestamp()), Member: id}).Err(); err != nil {
		common.SysError("invoice queue scheduling deferred: " + err.Error())
	}
	return PreviewInvoiceMailBatch(actorID, id)
}

func PauseInvoiceMailBatch(ctx context.Context, actorID int, id string) (*InvoiceMailPreview, error) {
	if _, _, err := LoadInvoiceMailBatch(actorID, id); err != nil {
		return nil, err
	}
	if err := model.DB.WithContext(ctx).Model(&model.InvoiceMailBatch{}).Where("id = ? AND queue_status IN ?", id, []string{"queued", "running"}).Update("queue_status", "paused").Error; err != nil {
		return nil, err
	}
	return PreviewInvoiceMailBatch(actorID, id)
}

var mailWorkerOnce sync.Once

func StartMailQueues() {
	mailWorkerOnce.Do(func() {
		if !common.RedisEnabled || common.RDB == nil {
			return
		}
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			var lastScan time.Time
			workers := []func(context.Context) error{runEmailNotificationOnce, runInvoiceMailQueueOnce}
			first := 0
			for range ticker.C {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				if time.Since(lastScan) >= 15*time.Second {
					if err := reconcileInvoiceMailQueue(ctx); err != nil {
						common.SysError("invoice queue recovery failed: " + err.Error())
					}
					if err := maintainEmailNotifications(ctx); err != nil {
						common.SysError("email notification maintenance failed: " + err.Error())
					}
					lastScan = time.Now()
				}
				for i := range workers {
					if err := workers[(first+i)%len(workers)](ctx); err != nil {
						common.SysError("mail queue worker failed: " + err.Error())
					}
				}
				first = (first + 1) % len(workers)
				cancel()
			}
		}()
	})
}

func reconcileInvoiceMailQueue(ctx context.Context) error {
	var batches []model.InvoiceMailBatch
	if err := model.DB.WithContext(ctx).Select("id, queue_next_at").Where("queue_status IN ?", []string{"queued", "running"}).Order("queue_next_at ASC").Limit(200).Find(&batches).Error; err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}
	items := make([]*redis.Z, 0, len(batches))
	for _, batch := range batches {
		items = append(items, &redis.Z{Score: float64(batch.QueueNextAt), Member: batch.ID})
	}
	return common.RDB.ZAdd(ctx, invoiceMailQueueKey(), items...).Err()
}

var invoiceMailPop = redis.NewScript(`local rows=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',ARGV[1],'LIMIT',0,1); if #rows == 0 then return '' end redis.call('ZREM',KEYS[1],rows[1]); return rows[1]`)

func runInvoiceMailQueueOnce(ctx context.Context) error {
	due, err := common.RDB.ZCount(ctx, invoiceMailQueueKey(), "-inf", strconv.FormatInt(common.GetTimestamp(), 10)).Result()
	if err != nil || due == 0 {
		return err
	}
	lease, err := common.AcquireEmailLease(ctx, false)
	if errors.Is(err, common.ErrEmailQueueBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	delay := 10 * time.Second
	var sendErr error
	defer func() { lease.Release(sendErr) }()
	id, err := invoiceMailPop.Run(ctx, common.RDB, []string{invoiceMailQueueKey()}, common.GetTimestamp()).Text()
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	var queue InvoiceMailQueue
	var payload string
	var batch model.InvoiceMailBatch
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockInvoiceMailBatch(tx, id)
		if err != nil {
			return err
		}
		batch = *current
		if batch.QueueStatus != "queued" && batch.QueueStatus != "running" {
			return model.ErrInvoiceMail
		}
		if batch.QueueNextAt > common.GetTimestamp() {
			return model.ErrInvoiceMail
		}
		if err := common.UnmarshalJsonStr(batch.QueuePayload, &queue); err != nil {
			return err
		}
		if queue.Position < 0 || queue.Position >= len(queue.Items) {
			return model.ErrInvoiceMail
		}
		payload = batch.QueuePayload
		return tx.Model(current).Update("queue_status", "running").Error
	})
	if err != nil {
		return err
	}
	item := queue.Items[queue.Position]
	sendErr = sendInvoiceMailBatchItem(ctx, queue.ActorID, id, item.TicketID, item.Review, lease)
	queue.Attempts++
	status, nextAt := "queued", common.GetTimestamp()+int64(delay/time.Second)
	if sendErr == nil {
		queue.Position++
		queue.Attempts = 0
		queue.Reason = ""
		if queue.Position == len(queue.Items) {
			status = "completed"
		}
	} else {
		common.SysError(fmt.Sprintf("invoice queue delivery failed: batch=%s ticket=%d attempt=%d: %v", id, item.TicketID, queue.Attempts, sendErr))
		status = "paused"
		queue.Reason = "Invoice delivery failed; refresh the batch before retrying"
		var receipt model.InvoiceMailDelivery
		var manifest InvoiceMailManifest
		if common.UnmarshalJsonStr(batch.Manifest, &manifest) == nil {
			for _, group := range manifest.Groups {
				if group.TicketID == item.TicketID {
					if err := model.DB.Where("delivery_key = ?", group.DeliveryKey).First(&receipt).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
						return err
					}
					break
				}
			}
		}
		retry := common.SMTPRetryDelay(sendErr, queue.Attempts)
		if receipt.State == "failed" && retry > 0 {
			queue.Items[queue.Position].Review = ""
			status = "queued"
			queue.Reason = "SMTP retry scheduled"
			nextAt = common.GetTimestamp() + int64(retry/time.Second)
		}
	}
	encoded, err := common.Marshal(queue)
	if err != nil {
		return err
	}
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockInvoiceMailBatch(tx, id)
		if err != nil {
			return err
		}
		if current.QueuePayload != payload {
			return nil
		}
		if current.QueueStatus == "paused" {
			status = "paused"
		}
		return tx.Model(current).Updates(map[string]any{"queue_status": status, "queue_next_at": nextAt, "queue_payload": string(encoded)}).Error
	})
	if err != nil {
		return err
	}
	if status == "queued" {
		return common.RDB.ZAdd(ctx, invoiceMailQueueKey(), &redis.Z{Score: float64(nextAt), Member: id}).Err()
	}
	return nil
}
