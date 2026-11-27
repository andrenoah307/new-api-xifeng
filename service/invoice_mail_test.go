package service

import (
	"context"
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
