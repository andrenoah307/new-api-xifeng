package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceQueueFIFOAndCancellation(t *testing.T) {
	var q financeQueue
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, err := q.enqueue(ctx, "first", 1, false, cancel)
	require.NoError(t, err)
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	second, err := q.enqueue(secondCtx, "second", 2, true, cancelSecond)
	require.NoError(t, err)
	third, err := q.enqueue(ctx, "third", 3, true, func() {})
	require.NoError(t, err)
	assert.Equal(t, "running", q.list(1, true, "")[0].State)
	assert.Equal(t, 1, q.list(2, false, "")[0].Ahead)
	assert.Empty(t, q.list(1, false, ""), "demoted users cannot inspect admin tasks")
	assert.False(t, q.cancelTask(9, "second"))
	assert.True(t, q.cancelTask(2, "second"))
	assert.ErrorIs(t, secondCtx.Err(), context.Canceled)
	assert.Empty(t, q.list(2, false, ""))
	q.finish(second)
	assert.Equal(t, 1, q.list(3, false, "")[0].Ahead)
	assert.True(t, q.cancelTask(1, "first"))
	assert.Equal(t, "cancelling", q.list(1, true, "")[0].State)
	select {
	case <-third.ready:
		t.Fatal("running cancellation must not release the slot before its query exits")
	default:
	}
	q.finish(first)
	select {
	case <-third.ready:
	default:
		t.Fatal("next FIFO request was not admitted")
	}
	assert.Equal(t, "running", q.list(3, false, "")[0].State)
	q.finish(third)
	assert.Empty(t, q.list(3, false, ""))
}

func TestFinanceQueueBoundsAndRequestCancellation(t *testing.T) {
	var q financeQueue
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, err := q.enqueue(ctx, "a", 1, true, cancel)
	require.NoError(t, err)
	second, err := q.enqueue(ctx, "b", 1, true, cancel)
	require.NoError(t, err)
	_, err = q.enqueue(ctx, "c", 1, true, cancel)
	require.Error(t, err, "one user must not fill the queue")
	_, err = q.enqueue(ctx, "a", 2, true, cancel)
	require.Error(t, err, "active task IDs cannot be reused")
	cancel()
	_, err = q.enqueue(ctx, "d", 2, true, cancel)
	require.ErrorIs(t, err, context.Canceled)
	q.finish(second)
	q.finish(first)
	assert.Empty(t, q.list(1, true, ""))
}

func TestFinanceTaskAPIEnforcesFreshOwnerIdentity(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "finance-queue-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task, err := channelFinanceQueue.enqueue(ctx, "owned-task", user.Id, true, cancel)
	require.NoError(t, err)
	defer channelFinanceQueue.finish(task)
	call := func(handler gin.HandlerFunc, method string, id int) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(method, "/?task_id=owned-task", nil)
		c.Set("id", id)
		c.Set("role", common.RoleCommonUser)
		handler(c)
		return rec
	}
	assert.Contains(t, call(GetFinanceTasks, http.MethodGet, user.Id).Body.String(), "owned-task")
	other := model.User{Username: "other-queue-user", AffCode: "queue-other", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&other).Error)
	assert.NotContains(t, call(GetFinanceTasks, http.MethodGet, other.Id).Body.String(), "owned-task")
	assert.Contains(t, call(CancelFinanceTask, http.MethodPost, other.Id).Body.String(), `"cancelled":false`)
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusDisabled).Error)
	assert.Equal(t, http.StatusServiceUnavailable, call(GetFinanceTasks, http.MethodGet, user.Id).Code)
	assert.NoError(t, ctx.Err())
	require.NoError(t, db.Model(&user).Update("status", common.UserStatusEnabled).Error)
	assert.Contains(t, call(CancelFinanceTask, http.MethodPost, user.Id).Body.String(), `"cancelled":true`)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestFinanceRequestQueuesAndExplicitCancellationIsTerminal(t *testing.T) {
	db := setupLogExportValidationDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	user := model.User{Username: "queued-finance-request", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	head, err := channelFinanceQueue.enqueue(context.Background(), "blocking-request", user.Id+1, true, func() {})
	require.NoError(t, err)
	defer channelFinanceQueue.finish(head)
	const id = "11111111-1111-4111-8111-111111111111"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_timestamp=5&end_timestamp=9&task_id="+id, nil)
	c.Set("id", user.Id)
	c.Set("role", user.Role)
	done := make(chan struct{})
	go func() { defer close(done); GetChannelFinanceStats(c) }()
	t.Cleanup(func() { channelFinanceQueue.cancelTask(user.Id, id); <-done })
	require.Eventually(t, func() bool { return len(channelFinanceQueue.list(user.Id, true, id)) == 1 }, time.Second, time.Millisecond)
	assert.Equal(t, "queued", channelFinanceQueue.list(user.Id, true, id)[0].State)
	require.True(t, channelFinanceQueue.cancelTask(user.Id, id))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled request did not exit")
	}
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"cancelled":true`)
	assert.NotContains(t, rec.Body.String(), "channels")
	assert.Empty(t, channelFinanceQueue.list(user.Id, true, id))
}
