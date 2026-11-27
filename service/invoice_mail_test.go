package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/setting"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceMailCleanupAndDraftOwnership(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.InvoiceMailBatch{}).Error)
		require.NoError(t, model.DB.Where("1 = 1").Delete(&model.InvoiceMailDelivery{}).Error)
	})
	oldKind, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage = "local"
	setting.TicketAttachmentLocalPath = t.TempDir()
	t.Cleanup(func() { setting.TicketAttachmentStorage = oldKind; setting.TicketAttachmentLocalPath = oldPath })
	storage, err := attachment.Current()
	require.NoError(t, err)
	ctx := context.Background()
	key := "invoice-mail/" + uuid.NewString() + ".png"
	require.NoError(t, storage.Put(ctx, key, strings.NewReader("data"), 4, "image/png"))
	manifest := InvoiceMailManifest{Storage: "local", StorageLocation: invoiceMailStorageLocation(), Groups: []InvoiceMailGroup{{TicketID: 1, DeliveryKey: strings.Repeat("a", 64), Files: []InvoiceMailFile{{Key: key}}}}}
	encoded, err := common.Marshal(manifest)
	require.NoError(t, err)
	batch := model.InvoiceMailBatch{ID: uuid.NewString(), OwnerID: 930010, ExpiresAt: common.GetTimestamp() + 86400, Manifest: string(encoded)}
	require.NoError(t, model.DB.Create(&batch).Error)
	for _, user := range []model.User{{Id: 930010, Username: "uploader", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}, {Id: 930011, Username: "other-finance", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}, {Id: 930012, Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}} {
		user.AffCode = user.Username
		require.NoError(t, model.DB.Create(&user).Error)
	}
	_, _, err = LoadInvoiceMailBatch(930010, batch.ID)
	require.NoError(t, err)
	_, _, err = LoadInvoiceMailBatch(930011, batch.ID)
	require.Error(t, err)
	_, _, err = LoadInvoiceMailBatch(930012, batch.ID)
	require.NoError(t, err)
	require.NoError(t, CleanupInvoiceMailBatch(ctx, &batch, &manifest, false))
	exists, err := storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
	delivery := model.InvoiceMailDelivery{TicketID: 1, DeliveryKey: strings.Repeat("a", 64), State: "sent"}
	require.NoError(t, model.DB.Create(&delivery).Error)
	oldRoot := setting.TicketAttachmentLocalPath
	setting.TicketAttachmentLocalPath = t.TempDir()
	require.Error(t, CleanupInvoiceMailBatch(ctx, &batch, &manifest, false))
	setting.TicketAttachmentLocalPath = oldRoot
	require.NoError(t, CleanupInvoiceMailBatch(ctx, &batch, &manifest, false))
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, model.DB.First(&batch, "id = ?", batch.ID).Error)
	assert.Positive(t, batch.CleanedAt)
	require.NoError(t, model.DB.First(&delivery, delivery.ID).Error)
	assert.Equal(t, "sent", delivery.State)
	// Expiration removes abandoned files too, but never an active send.
	require.NoError(t, storage.Put(ctx, key, strings.NewReader("data"), 4, "image/png"))
	batch.CleanedAt = 0
	require.NoError(t, model.DB.Model(&batch).Update("cleaned_at", 0).Error)
	require.NoError(t, model.DB.Model(&delivery).Updates(map[string]any{"state": "sending", "updated_at": common.GetTimestamp()}).Error)
	require.NoError(t, CleanupInvoiceMailBatch(ctx, &batch, &manifest, true))
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
	require.NoError(t, model.DB.Model(&delivery).Update("state", "failed").Error)
	require.NoError(t, CleanupInvoiceMailBatch(ctx, &batch, &manifest, true))
	exists, err = storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestInvoiceMailCancelRevokesBeforeCleanupAndPreservesReceipts(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}))
	actor := model.User{Id: 930020, Username: "cancel-finance", AffCode: "cancel-finance", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	other := model.User{Id: 930021, Username: "cancel-other", AffCode: "cancel-other", Role: common.RoleFinanceUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&other).Error)
	oldKind, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = "local", t.TempDir()
	t.Cleanup(func() {
		setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = oldKind, oldPath
		require.NoError(t, model.DB.Where("owner_id = ?", actor.Id).Delete(&model.InvoiceMailBatch{}).Error)
		require.NoError(t, model.DB.Where("ticket_id = ?", 930020).Delete(&model.InvoiceMailDelivery{}).Error)
	})
	storage, err := attachment.Current()
	require.NoError(t, err)
	ctx := context.Background()
	key := "invoice-mail/" + uuid.NewString() + ".png"
	require.NoError(t, storage.Put(ctx, key, strings.NewReader("data"), 4, "image/png"))
	delivery := model.InvoiceMailDelivery{TicketID: 930020, DeliveryKey: strings.Repeat("c", 64), State: "unknown"}
	require.NoError(t, model.DB.Create(&delivery).Error)
	manifest := InvoiceMailManifest{Storage: "local", StorageLocation: invoiceMailStorageLocation(), Groups: []InvoiceMailGroup{{TicketID: delivery.TicketID, DeliveryKey: delivery.DeliveryKey, Files: []InvoiceMailFile{{Key: key}}}}}
	encoded, err := common.Marshal(manifest)
	require.NoError(t, err)
	batch := model.InvoiceMailBatch{ID: uuid.NewString(), OwnerID: actor.Id, CreatedAt: common.GetTimestamp(), ExpiresAt: common.GetTimestamp() + 86400, Manifest: string(encoded)}
	require.NoError(t, model.DB.Create(&batch).Error)
	_, err = CancelInvoiceMailBatch(ctx, other.Id, batch.ID)
	require.Error(t, err)
	// Existing rows can receive NULL when the additive column is migrated.
	require.NoError(t, model.DB.Model(&batch).Update("cancelled_at", nil).Error)
	// Storage configuration changes must leave revocation committed and cleanup pending.
	batchStoragePath := setting.TicketAttachmentLocalPath
	setting.TicketAttachmentLocalPath = t.TempDir()
	preview, err := CancelInvoiceMailBatch(ctx, actor.Id, batch.ID)
	require.NoError(t, err)
	assert.Positive(t, preview.CancelledAt)
	assert.Zero(t, preview.CleanedAt)
	require.Error(t, SendInvoiceMailBatchItem(ctx, actor.Id, batch.ID, delivery.TicketID, ""))
	setting.TicketAttachmentLocalPath = batchStoragePath
	CleanupExpiredInvoiceMailBatches(ctx)
	preview, err = PreviewInvoiceMailBatch(actor.Id, batch.ID)
	require.NoError(t, err)
	assert.Positive(t, preview.CleanedAt)
	exists, err := storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, model.DB.First(&delivery, delivery.ID).Error)
	assert.Equal(t, "unknown", delivery.State)
	_, err = CancelInvoiceMailBatch(ctx, actor.Id, batch.ID)
	require.NoError(t, err)
}

func TestInvoiceMailCleanupReachesExpiredBatchesAfterRetainedPage(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.InvoiceMailBatch{}, &model.InvoiceMailDelivery{}))
	oldKind, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = "local", t.TempDir()
	t.Cleanup(func() {
		setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = oldKind, oldPath
		require.NoError(t, model.DB.Where("owner_id = ?", 930099).Delete(&model.InvoiceMailBatch{}).Error)
	})
	storage, err := attachment.Current()
	require.NoError(t, err)
	ctx := context.Background()
	key := "invoice-mail/expired.png"
	require.NoError(t, storage.Put(ctx, key, strings.NewReader("data"), 4, "image/png"))
	manifest := InvoiceMailManifest{Storage: "local", StorageLocation: invoiceMailStorageLocation(), Groups: []InvoiceMailGroup{{DeliveryKey: strings.Repeat("e", 64), Files: []InvoiceMailFile{{Key: key}}}}}
	encoded, err := common.Marshal(manifest)
	require.NoError(t, err)
	var batches []model.InvoiceMailBatch
	// Exactly one page of retained drafts precedes the expired batch.
	for i := 0; i < 200; i++ {
		batches = append(batches, model.InvoiceMailBatch{ID: fmt.Sprintf("retained-%03d", i), OwnerID: 930099, ExpiresAt: common.GetTimestamp() + 86400, Manifest: string(encoded)})
	}
	batches = append(batches, model.InvoiceMailBatch{ID: "z-expired", OwnerID: 930099, ExpiresAt: common.GetTimestamp() - 1, Manifest: string(encoded)})
	require.NoError(t, model.DB.Create(&batches).Error)
	CleanupExpiredInvoiceMailBatches(ctx)
	var expired model.InvoiceMailBatch
	require.NoError(t, model.DB.First(&expired, "id = ?", "z-expired").Error)
	assert.Positive(t, expired.CleanedAt)
	exists, err := storage.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
	var retained int64
	require.NoError(t, model.DB.Model(&model.InvoiceMailBatch{}).Where("owner_id = ? AND cleaned_at = ?", 930099, 0).Count(&retained).Error)
	assert.EqualValues(t, 200, retained)
}
