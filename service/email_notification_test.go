package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEmailNotificationTable(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.EmailNotification{}))
	t.Cleanup(func() { require.NoError(t, model.DB.Where("1 = 1").Delete(&model.EmailNotification{}).Error) })
}

func TestNotificationAndInvoiceShareAccountPacing(t *testing.T) {
	actor, batch, mr, count := invoiceQueueFixture(t, "success")
	setupEmailNotificationTable(t)
	ctx := context.Background()
	_, err := EnqueueInvoiceMailBatch(ctx, actor, batch.ID, 0, "")
	require.NoError(t, err)
	// Redis unavailable at enqueue does not lose a persisted notification.
	redis := common.RDB
	common.RDB = nil
	err = EnqueueEmailNotification("Ticket updated", "recipient@example.test", "Resolved")
	common.RDB = redis
	require.NoError(t, err)
	assert.Zero(t, count.Load())
	mr.FlushAll()
	require.NoError(t, reconcileInvoiceMailQueue(ctx))
	require.NoError(t, runEmailNotificationOnce(ctx))
	assert.EqualValues(t, 1, count.Load())
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 1, count.Load(), "invoices cannot bypass notification pacing")
	mr.FastForward(10 * time.Second)
	require.NoError(t, runInvoiceMailQueueOnce(ctx))
	assert.EqualValues(t, 2, count.Load())
	require.NoError(t, EnqueueEmailNotification("Top-up", "recipient@example.test", "Paid"))
	require.NoError(t, runEmailNotificationOnce(ctx))
	assert.EqualValues(t, 2, count.Load(), "notifications cannot bypass invoice pacing")
	mr.FastForward(10 * time.Second)
	require.NoError(t, runEmailNotificationOnce(ctx))
	assert.EqualValues(t, 3, count.Load())
	var jobs []model.EmailNotification
	require.NoError(t, model.DB.Find(&jobs).Error)
	require.Len(t, jobs, 2)
	for _, job := range jobs {
		assert.Equal(t, "sent", job.Status)
		assert.Empty(t, job.Payload)
	}
}

func TestEmailNotificationRetriesAndUnknownRecovery(t *testing.T) {
	for _, mode := range []string{"535", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			_, _, mr, count := invoiceQueueFixture(t, mode)
			setupEmailNotificationTable(t)
			ctx := context.Background()
			require.NoError(t, EnqueueEmailNotification("Ticket", "recipient@example.test", "Updated"))
			var job model.EmailNotification
			require.NoError(t, model.DB.First(&job).Error)
			require.NoError(t, runEmailNotificationOnce(ctx))
			require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
			if mode == "unknown" {
				assert.Equal(t, "unknown", job.Status)
				mr.FastForward(time.Hour)
				require.NoError(t, runEmailNotificationOnce(ctx))
				assert.EqualValues(t, 1, count.Load())
				return
			}
			assert.Equal(t, "pending", job.Status)
			assert.GreaterOrEqual(t, job.NextAt, common.GetTimestamp()+899)
			for attempt := 2; attempt <= 3; attempt++ {
				mr.FastForward(15 * time.Minute)
				require.NoError(t, model.DB.Model(&job).Update("next_at", 0).Error)
				require.NoError(t, runEmailNotificationOnce(ctx))
			}
			require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
			assert.Equal(t, "failed", job.Status)
			assert.Equal(t, 3, job.Attempts)
			assert.EqualValues(t, 3, count.Load())
		})
	}
}

func TestEmailNotificationStaleClaimsAndRetention(t *testing.T) {
	_, _, _, count := invoiceQueueFixture(t, "success")
	setupEmailNotificationTable(t)
	ctx := context.Background()
	require.NoError(t, EnqueueEmailNotification("Ticket", "recipient@example.test", "Private body"))
	var job model.EmailNotification
	require.NoError(t, model.DB.First(&job).Error)
	require.NoError(t, model.DB.Model(&job).Updates(map[string]any{"status": "sending", "updated_at": common.GetTimestamp() - 121}).Error)
	require.NoError(t, maintainEmailNotifications(ctx))
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Equal(t, "unknown", job.Status)
	require.NoError(t, runEmailNotificationOnce(ctx))
	assert.Zero(t, count.Load())
	require.NoError(t, model.DB.Model(&job).Update("created_at", common.GetTimestamp()-86401).Error)
	require.NoError(t, maintainEmailNotifications(ctx))
	require.NoError(t, model.DB.First(&job, "id = ?", job.ID).Error)
	assert.Empty(t, job.Payload)
	require.NoError(t, EnqueueEmailNotification("Ticket", "recipient@example.test", "Expired"))
	require.NoError(t, model.DB.Model(&model.EmailNotification{}).Where("status = ?", "pending").Update("created_at", common.GetTimestamp()-86401).Error)
	require.NoError(t, maintainEmailNotifications(ctx))
	require.NoError(t, runEmailNotificationOnce(ctx))
	assert.Zero(t, count.Load())
}
