package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/setting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvoiceMailFile struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type InvoiceMailGroup struct {
	TicketID    int               `json:"ticket_id"`
	Recipient   string            `json:"recipient"`
	DeliveryKey string            `json:"delivery_key"`
	Files       []InvoiceMailFile `json:"files"`
}

type InvoiceMailManifest struct {
	Config          model.InvoiceMailConfig `json:"config"`
	Storage         string                  `json:"storage"`
	StorageLocation string                  `json:"storage_location"`
	Groups          []InvoiceMailGroup      `json:"groups"`
	Ignored         []string                `json:"ignored"`
}

func invoiceMailStorageLocation() string {
	var location []string
	switch setting.TicketAttachmentStorage {
	case "", "local":
		root := setting.TicketAttachmentLocalPath
		if strings.TrimSpace(root) == "" {
			root = "data/ticket_attachments"
		}
		absolute, _ := filepath.Abs(root)
		location = []string{"local", absolute}
	case "s3":
		location = []string{"s3", setting.TicketAttachmentS3Endpoint, setting.TicketAttachmentS3Region, setting.TicketAttachmentS3Bucket}
	case "oss":
		location = []string{"oss", setting.TicketAttachmentOSSEndpoint, setting.TicketAttachmentOSSBucket}
	case "cos":
		location = []string{"cos", setting.TicketAttachmentCOSEndpoint, setting.TicketAttachmentCOSRegion, setting.TicketAttachmentCOSBucket}
	}
	encoded, _ := common.Marshal(location)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

type InvoiceMailItem struct {
	TicketID  int      `json:"ticket_id"`
	Subject   string   `json:"subject"`
	Recipient string   `json:"recipient"`
	Files     []string `json:"files"`
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
}

type InvoiceMailPreview struct {
	ID             string            `json:"id"`
	ExpiresAt      int64             `json:"expires_at"`
	CleanedAt      int64             `json:"cleaned_at"`
	CancelledAt    int64             `json:"cancelled_at"`
	RetentionHours int64             `json:"retention_hours"`
	Reply          string            `json:"reply"`
	Status         int               `json:"status"`
	Items          []InvoiceMailItem `json:"items"`
	Ignored        []string          `json:"ignored"`
	Queue          *InvoiceMailQueue `json:"queue,omitempty"`
}

// Uploads are drafts only. Files live in the configured private attachment
// storage so confirmation can reach a different application replica.
func CreateInvoiceMailBatch(ctx context.Context, actorID int, files []InvoiceArchiveFile, ignored []string) (*InvoiceMailPreview, error) {
	if _, err := model.AuthorizeInvoiceMail(actorID); err != nil {
		return nil, err
	}
	storage, err := attachment.Current()
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	manifest := InvoiceMailManifest{Config: model.GetInvoiceMailConfig(), Storage: storage.Kind(), StorageLocation: invoiceMailStorageLocation(), Ignored: ignored}
	batch := model.InvoiceMailBatch{ID: uuid.NewString(), OwnerID: actorID, CreatedAt: now, ExpiresAt: now + int64(manifest.Config.RetentionHours)*3600}
	var keys []string
	saved := false
	defer func() {
		if saved {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, key := range keys {
			_ = storage.Delete(cleanupCtx, key)
		}
	}()
	for _, file := range files {
		if len(manifest.Groups) == 0 || manifest.Groups[len(manifest.Groups)-1].TicketID != file.TicketID {
			_, recipient, _ := model.InvoiceMailRecipient(file.TicketID)
			manifest.Groups = append(manifest.Groups, InvoiceMailGroup{TicketID: file.TicketID, Recipient: recipient})
		}
		group := &manifest.Groups[len(manifest.Groups)-1]
		key := "invoice-mail/" + batch.ID + "/" + uuid.NewString() + ".png"
		keys = append(keys, key)
		reader, err := os.Open(file.Path)
		if err != nil {
			return nil, err
		}
		err = storage.Put(ctx, key, reader, file.Size, "image/png")
		_ = reader.Close()
		if err != nil {
			return nil, err
		}
		group.Files = append(group.Files, InvoiceMailFile{Key: key, Name: file.Name, Digest: file.Digest, Size: file.Size})
	}
	for i := range manifest.Groups {
		group := &manifest.Groups[i]
		var hashes []string
		for _, file := range group.Files {
			hashes = append(hashes, file.Digest)
		}
		group.DeliveryKey = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", group.TicketID, strings.Join(hashes, ":")))))
	}
	encoded, err := common.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	batch.Manifest = string(encoded)
	if err := model.DB.Create(&batch).Error; err != nil {
		return nil, err
	}
	saved = true
	if err := CleanupInvoiceMailBatch(ctx, &batch, &manifest, false); err != nil {
		return nil, err
	}
	return PreviewInvoiceMailBatch(actorID, batch.ID)
}

func LoadInvoiceMailBatch(actorID int, id string) (*model.InvoiceMailBatch, *InvoiceMailManifest, error) {
	actor, err := model.AuthorizeInvoiceMail(actorID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, nil, model.ErrInvoiceMail
	}
	var batch model.InvoiceMailBatch
	query := model.DB.Where("id = ?", id)
	if actor.Role != common.RoleRootUser {
		query = query.Where("owner_id = ?", actorID)
	}
	if err := query.First(&batch).Error; err != nil {
		return nil, nil, model.ErrInvoiceMail
	}
	var manifest InvoiceMailManifest
	if err := common.UnmarshalJsonStr(batch.Manifest, &manifest); err != nil {
		return nil, nil, err
	}
	return &batch, &manifest, nil
}

func PreviewInvoiceMailBatch(actorID int, id string) (*InvoiceMailPreview, error) {
	batch, manifest, err := LoadInvoiceMailBatch(actorID, id)
	if err != nil {
		return nil, err
	}
	preview := &InvoiceMailPreview{ID: batch.ID, ExpiresAt: batch.ExpiresAt, CleanedAt: batch.CleanedAt, Reply: manifest.Config.Reply, Status: manifest.Config.Status, Ignored: manifest.Ignored, Items: make([]InvoiceMailItem, 0, len(manifest.Groups))}
	preview.CancelledAt = batch.CancelledAt
	preview.RetentionHours = (batch.ExpiresAt - batch.CreatedAt) / 3600
	if batch.QueuePayload != "" {
		var queue InvoiceMailQueue
		if err := common.UnmarshalJsonStr(batch.QueuePayload, &queue); err != nil {
			return nil, err
		}
		queue.Status, queue.NextAt = batch.QueueStatus, batch.QueueNextAt
		preview.Queue = &queue
	}
	var deliveries []model.InvoiceMailDelivery
	var keys []string
	for _, group := range manifest.Groups {
		keys = append(keys, group.DeliveryKey)
	}
	if err := model.DB.Where("delivery_key IN ?", keys).Find(&deliveries).Error; err != nil {
		return nil, err
	}
	states := make(map[string]model.InvoiceMailDelivery, len(deliveries))
	for _, delivery := range deliveries {
		states[delivery.DeliveryKey] = delivery
	}
	for _, group := range manifest.Groups {
		item := InvoiceMailItem{TicketID: group.TicketID, Recipient: group.Recipient, State: "ready"}
		for index, file := range group.Files {
			item.Files = append(item.Files, fmt.Sprintf("%d. %s", index+1, file.Name))
		}
		ticket, recipient, validation := model.InvoiceMailRecipient(group.TicketID)
		if ticket != nil {
			item.Subject = ticket.Subject
		}
		if validation != nil || recipient != group.Recipient || recipient == "" {
			item.State = "blocked"
			item.Reason = "Invoice or recipient is unavailable"
			if errors.Is(validation, model.ErrTicketInvoiceEmailEmpty) {
				item.Reason = "No valid invoice email"
			}
			if errors.Is(validation, model.ErrRefundFinancePending) {
				item.Reason = "Invoice is locked by a refund"
			}
			if errors.Is(validation, model.ErrInvoiceTaxUnpaid) {
				item.Reason = "Invoice tax has not been paid"
			}
			if recipient != group.Recipient {
				item.Reason = "Recipient changed; upload again"
			}
		}
		if delivery, ok := states[group.DeliveryKey]; ok {
			if delivery.State == "sent" || delivery.State == "unknown" || delivery.State == "sending" || item.State != "blocked" {
				item.State = delivery.State
			}
			if item.State == "sending" && delivery.UpdatedAt < common.GetTimestamp()-120 {
				item.State = "unknown"
			}
		}
		preview.Items = append(preview.Items, item)
	}
	return preview, nil
}

func SendInvoiceMailBatchItem(ctx context.Context, actorID int, id string, ticketID int, review string) (err error) {
	if review == "delivered" {
		return sendInvoiceMailBatchItem(ctx, actorID, id, ticketID, review, nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	lease, err := common.AcquireEmailLease(ctx, true)
	if err != nil {
		return err
	}
	defer func() { lease.Release(err) }()
	return sendInvoiceMailBatchItem(ctx, actorID, id, ticketID, review, lease)
}

func sendInvoiceMailBatchItem(ctx context.Context, actorID int, id string, ticketID int, review string, lease *common.EmailLease) error {
	batch, manifest, err := LoadInvoiceMailBatch(actorID, id)
	if err != nil {
		return err
	}
	var selected *InvoiceMailGroup
	for i := range manifest.Groups {
		if manifest.Groups[i].TicketID == ticketID {
			selected = &manifest.Groups[i]
			break
		}
	}
	if selected == nil {
		return model.ErrInvoiceMail
	}
	var sent int64
	if err := model.DB.Model(&model.InvoiceMailDelivery{}).Where("delivery_key = ? AND state = ?", selected.DeliveryKey, "sent").Count(&sent).Error; err != nil {
		return err
	}
	if sent != 0 {
		return CleanupInvoiceMailBatch(ctx, batch, manifest, false)
	}
	if review != "delivered" && (batch.CancelledAt != 0 || batch.CleanedAt != 0 || batch.ExpiresAt <= common.GetTimestamp()) {
		return model.ErrInvoiceMail
	}
	storage, err := attachment.Current()
	if err != nil || storage.Kind() != manifest.Storage || manifest.StorageLocation != invoiceMailStorageLocation() {
		return model.ErrInvoiceMail
	}
	var attachments []common.EmailAttachment
	if review != "delivered" {
		for _, file := range selected.Files {
			reader, err := storage.Get(ctx, file.Key)
			if err != nil {
				return model.ErrInvoiceMail
			}
			data, err := io.ReadAll(io.LimitReader(reader, 10*1024*1024+1))
			_ = reader.Close()
			if err != nil || int64(len(data)) != file.Size || fmt.Sprintf("%x", sha256.Sum256(data)) != file.Digest {
				return model.ErrInvoiceMail
			}
			attachments = append(attachments, common.EmailAttachment{Name: file.Name, Data: data})
		}
	}
	err = model.DeliverInvoiceMail(batch.ID, ticketID, actorID, selected.DeliveryKey, selected.Recipient, manifest.Config, review, func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if lease != nil {
			return common.SendEmailWithLease(ctx, lease, fmt.Sprintf("Invoice #%d", ticketID), selected.Recipient, "<p>"+html.EscapeString(manifest.Config.Reply)+"</p>", attachments)
		}
		return false, common.ErrEmailQueueBusy
	})
	if err != nil {
		return err
	}
	return CleanupInvoiceMailBatch(ctx, batch, manifest, false)
}

// Successful batches are cleaned immediately. Drafts expire at their snapshotted
// retention deadline; delivery receipts remain for deduplication and review.
func CleanupInvoiceMailBatch(ctx context.Context, batch *model.InvoiceMailBatch, manifest *InvoiceMailManifest, expired bool) error {
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := model.LockInvoiceMailBatch(tx, batch.ID)
		if err != nil {
			return err
		}
		*batch = *current
		if batch.CleanedAt != 0 {
			return nil
		}
		var keys []string
		for _, group := range manifest.Groups {
			keys = append(keys, group.DeliveryKey)
		}
		var count int64
		if expired || batch.CancelledAt != 0 || batch.ExpiresAt <= common.GetTimestamp() {
			if err := tx.Model(&model.InvoiceMailDelivery{}).Where("delivery_key IN ? AND state = ? AND updated_at > ?", keys, "sending", common.GetTimestamp()-120).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return nil
			}
		} else {
			if err := tx.Model(&model.InvoiceMailDelivery{}).Where("delivery_key IN ? AND state = ?", keys, "sent").Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(keys)) {
				return nil
			}
		}
		storage, err := attachment.Current()
		if err != nil || storage.Kind() != manifest.Storage || manifest.StorageLocation != invoiceMailStorageLocation() {
			return model.ErrInvoiceMail
		}
		for _, group := range manifest.Groups {
			for _, file := range group.Files {
				if err := storage.Delete(ctx, file.Key); err != nil {
					return err
				}
			}
		}
		return tx.Model(batch).Update("cleaned_at", common.GetTimestamp()).Error
	})
}

func CancelInvoiceMailBatch(ctx context.Context, actorID int, id string) (*InvoiceMailPreview, error) {
	batch, manifest, err := LoadInvoiceMailBatch(actorID, id)
	if err != nil {
		return nil, err
	}
	// Commit revocation before deleting files: storage failures must never make
	// an unwanted batch sendable again. This update waits for an active SMTP lock.
	if err := model.DB.WithContext(ctx).Model(&model.InvoiceMailBatch{}).Where("id = ? AND (cancelled_at = ? OR cancelled_at IS NULL)", id, 0).Updates(map[string]any{"cancelled_at": common.GetTimestamp(), "queue_status": "paused"}).Error; err != nil {
		return nil, err
	}
	if err := CleanupInvoiceMailBatch(ctx, batch, manifest, true); err != nil {
		common.SysError("cancelled invoice batch cleanup pending: " + err.Error())
	}
	return PreviewInvoiceMailBatch(actorID, id)
}

func CleanupExpiredInvoiceMailBatches(ctx context.Context) {
	// Keyset pagination prevents retained drafts or storage failures in the first
	// page from permanently starving later expired/cancelled batches.
	lastID := ""
	for ctx.Err() == nil {
		var batches []model.InvoiceMailBatch
		if err := model.DB.WithContext(ctx).Where("cleaned_at = ? AND id > ?", 0, lastID).Order("id ASC").Limit(200).Find(&batches).Error; err != nil {
			common.SysError("invoice batch cleanup query failed: " + err.Error())
			return
		}
		if len(batches) == 0 {
			return
		}
		for _, batch := range batches {
			lastID = batch.ID
			var manifest InvoiceMailManifest
			if common.UnmarshalJsonStr(batch.Manifest, &manifest) != nil {
				continue
			}
			if err := CleanupInvoiceMailBatch(ctx, &batch, &manifest, batch.ExpiresAt <= common.GetTimestamp()); err != nil {
				common.SysError("invoice batch cleanup failed: " + err.Error())
			}
			if ctx.Err() != nil {
				return
			}
		}
	}
}
