package service

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestTaskSettlementRetainsLiabilityUntilFundingSucceeds(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RefundLiability{}, &model.TicketRefund{}, &model.Option{}))
	t.Cleanup(func() { require.NoError(t, model.DB.Where("1 = 1").Delete(&model.RefundLiability{}).Error) })
	seedUser(t, 995001, 8000)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 995001).Update("used_quota", 2000).Error)
	liability, err := model.BeginRefundLiability(995001, "failed-task-request")
	require.NoError(t, err)
	model.HoldRefundLiability(liability)
	model.FinishRefundLiabilityRequest(liability.ID)
	task := makeTask(995001, 0, 2000, 0, BillingSourceWallet, 0)
	task.PrivateData.RefundLiabilityID = liability.ID
	require.NoError(t, model.DB.Create(task).Error)
	injected := errors.New("wallet unavailable")
	callback := "test:refund_liability_funding_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "users" {
			tx.AddError(injected)
		}
	}))
	RefundTaskQuota(context.Background(), task, "failed")
	require.NoError(t, model.DB.Callback().Update().Remove(callback))
	var count int64
	require.NoError(t, model.DB.Model(&model.RefundLiability{}).Where("id = ?", liability.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	var owner model.User
	require.NoError(t, model.DB.First(&owner, 995001).Error)
	assert.Equal(t, 8000, owner.Quota)
	RefundTaskQuota(context.Background(), task, "failed")
	require.NoError(t, model.DB.First(&owner, 995001).Error)
	assert.Equal(t, 10000, owner.Quota)
	assert.Zero(t, owner.UsedQuota)
	require.NoError(t, model.DB.Model(&model.RefundLiability{}).Where("id = ?", liability.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingSettlementFailureMarksRequestUncertain(t *testing.T) {
	liability := &common.RefundLiability{ID: "test-request"}
	info := &relaycommon.RelayInfo{UserId: 995002, RefundLiability: liability, IsPlayground: true, BillingSource: BillingSourceSubscription}
	err := SettleBilling(&gin.Context{}, info, 100)
	require.Error(t, err)
	assert.True(t, liability.Failed.Load())
}
