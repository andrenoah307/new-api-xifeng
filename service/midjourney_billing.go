package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// RefundMidjourneyTokenQuota releases the token-side reservation for a
// failed legacy Midjourney task. TokenId is intentionally mandatory: for old
// rows without it we warn and leave the token accounting untouched instead of
// guessing from user or channel context.
func RefundMidjourneyTokenQuota(ctx context.Context, task *model.Midjourney) error {
	if task == nil || task.Quota <= 0 {
		return nil
	}
	if task.TokenId <= 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Midjourney task %s has no TokenId; token refund skipped", task.MjId))
		return nil
	}
	token, err := model.GetTokenById(task.TokenId)
	if err != nil {
		return err
	}
	return model.AdjustTokenQuota(task.TokenId, token.Key, -task.Quota, task.TokenPeriodStartAt, nil)
}
