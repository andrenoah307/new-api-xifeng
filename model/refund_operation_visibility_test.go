package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefundOperationLogsStayPrivate(t *testing.T) {
	truncateTables(t)
	rows := []*Log{
		{UserId: 71, Type: LogTypeManage, Content: "Refund #8 manual adjustment: private evidence"},
		{UserId: 71, Type: LogTypeManage, Content: "Refund payout #9 claimed"},
		{UserId: 71, Type: LogTypeManage, Content: "管理员批准退款，核销已扣余额 10"},
		{UserId: 71, Type: LogTypeManage, Content: "被推荐人退款，扣除推荐人奖励"},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)
	logs, total, err := GetUserLogs(context.Background(), 71, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, logs, 2)
	var contents []string
	for _, entry := range logs {
		contents = append(contents, entry.Content)
	}
	assert.ElementsMatch(t, []string{"退款通过，核销已扣余额 10", "被推荐人退款，扣除推荐人奖励"}, contents)
	var exported []*Log
	err = ExportUserLogs(context.Background(), 71, LogTypeUnknown, 0, 0, "", "", "", "", "", func(batch []*Log) error { exported = append(exported, batch...); return nil })
	require.NoError(t, err)
	require.Len(t, exported, 2)
	for _, entry := range exported {
		assert.NotContains(t, entry.Content, "管理员批准退款")
	}
	var stored Log
	require.NoError(t, LOG_DB.First(&stored, rows[2].Id).Error)
	assert.Equal(t, "管理员批准退款，核销已扣余额 10", stored.Content)
}

func TestRefundInternalAuditFailureRollsBackCompletion(t *testing.T) {
	f, ticket, _ := newRefundAdjustmentFixture(t)
	failure := errors.New("internal audit unavailable")
	name := "test:refund-internal-audit-failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { DB.Callback().Create().Remove(name) })
	_, _, _, _, err := UpdateRefundStatus(UpdateRefundStatusParams{OriginalWorkflow: true, TicketId: ticket.Id, AdminId: f.admin.Id, RefundStatus: RefundStatusRejected})
	require.ErrorIs(t, err, failure)
	var refund TicketRefund
	require.NoError(t, DB.Where("ticket_id = ?", ticket.Id).First(&refund).Error)
	assert.Equal(t, RefundStatusPending, refund.RefundStatus)
	assert.Equal(t, 17500000, refund.FrozenQuota)
	var owner User
	require.NoError(t, DB.First(&owner, f.user.Id).Error)
	assert.Equal(t, 500000, owner.Quota)
}
