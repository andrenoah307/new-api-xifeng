package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type emailNotificationPayload struct {
	Subject  string `json:"subject"`
	Receiver string `json:"receiver"`
	Content  string `json:"content"`
}

// EnqueueEmailNotification reports durable acceptance, not SMTP delivery.
// Synchronous/security-sensitive callers continue using common.SendEmail.
func EnqueueEmailNotification(subject, receiver, content string) error {
	if err := common.ValidateEmailMessage(subject, receiver, content); err != nil {
		return err
	}
	if !common.RedisEnabled {
		return common.SendEmail(subject, receiver, content)
	}
	encoded, err := common.Marshal(emailNotificationPayload{Subject: subject, Receiver: receiver, Content: content})
	if err != nil {
		return err
	}
	if len(encoded) > 4<<20 {
		return errors.New("email notification is too large")
	}
	now := common.GetTimestamp()
	job := model.EmailNotification{ID: uuid.NewString(), Status: "pending", NextAt: now, CreatedAt: now, UpdatedAt: now, Payload: string(encoded)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return model.DB.WithContext(ctx).Create(&job).Error
}

func maintainEmailNotifications(ctx context.Context) error {
	now := common.GetTimestamp()
	// A process may have died after DATA but before its DB acknowledgement.
	// Stale claims must never be replayed automatically.
	stale := model.DB.WithContext(ctx).Model(&model.EmailNotification{}).Where("status = ? AND updated_at < ?", "sending", now-120).Updates(map[string]any{"status": "unknown", "updated_at": now})
	if stale.Error != nil {
		return stale.Error
	}
	if stale.RowsAffected > 0 {
		common.SysError(fmt.Sprintf("email notifications require delivery review: count=%d", stale.RowsAffected))
	}
	expired := model.DB.WithContext(ctx).Model(&model.EmailNotification{}).Where("created_at < ? AND status = ?", now-86400, "pending").Update("status", "failed")
	if expired.Error != nil {
		return expired.Error
	}
	if expired.RowsAffected > 0 {
		common.SysError(fmt.Sprintf("email notifications expired without delivery: count=%d", expired.RowsAffected))
	}
	if err := model.DB.WithContext(ctx).Model(&model.EmailNotification{}).Where("created_at < ? AND status <> ? AND payload <> ?", now-86400, "sending", "").Update("payload", "").Error; err != nil {
		return err
	}
	return model.DB.WithContext(ctx).Where("created_at < ? AND status <> ?", now-7*86400, "sending").Delete(&model.EmailNotification{}).Error
}

func runEmailNotificationOnce(ctx context.Context) error {
	now := common.GetTimestamp()
	var job model.EmailNotification
	err := model.DB.WithContext(ctx).Where("status = ? AND next_at <= ? AND created_at >= ?", "pending", now, now-86400).Order("next_at ASC, created_at ASC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	lease, err := common.AcquireEmailLease(ctx, false)
	if errors.Is(err, common.ErrEmailQueueBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	var sendErr error
	defer func() { lease.Release(sendErr) }()
	attempt := uuid.NewString()
	claim := model.DB.WithContext(ctx).Model(&model.EmailNotification{}).Where("id = ? AND status = ? AND attempts = ?", job.ID, "pending", job.Attempts).Updates(map[string]any{"status": "sending", "attempt_id": attempt, "attempts": job.Attempts + 1, "updated_at": now})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil
	}
	var payload emailNotificationPayload
	sendErr = common.UnmarshalJsonStr(job.Payload, &payload)
	uncertain := false
	if sendErr == nil {
		uncertain, sendErr = common.SendEmailWithLease(ctx, lease, payload.Subject, payload.Receiver, payload.Content, nil)
	}
	status, nextAt, body := "sent", now, ""
	if sendErr != nil {
		status, body = "failed", job.Payload
		if uncertain {
			status = "unknown"
		} else if retry := common.SMTPRetryDelay(sendErr, job.Attempts+1); retry > 0 {
			status, nextAt = "pending", common.GetTimestamp()+int64(retry/time.Second)
		}
		common.SysError(fmt.Sprintf("email notification failed: job=%s attempt=%d status=%s: %v", job.ID, job.Attempts+1, status, sendErr))
	}
	// Use a fresh context to record DATA acceptance even if the caller expired.
	finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := model.DB.WithContext(finish).Model(&model.EmailNotification{}).Where("id = ? AND status = ? AND attempt_id = ?", job.ID, "sending", attempt).Updates(map[string]any{"status": status, "next_at": nextAt, "updated_at": common.GetTimestamp(), "payload": body})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("email notification acknowledgement lost")
	}
	return nil
}
