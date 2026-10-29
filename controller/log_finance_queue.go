package controller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
)

type financeTask struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Ahead     int    `json:"ahead"`
	CreatedAt int64  `json:"created_at"`
	owner     int
	personal  bool
	ready     chan struct{}
	cancel    context.CancelFunc
}

// Request-owned tasks retain no credentials or results. Running cancellations
// keep their slot until the database call returns, preventing overlapping work.
type financeQueue struct {
	mu    sync.Mutex
	tasks []*financeTask
}

var channelFinanceQueue financeQueue

func (q *financeQueue) enqueue(ctx context.Context, id string, owner int, personal bool, cancel context.CancelFunc) (*financeTask, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owned := 0
	for _, task := range q.tasks {
		if task.ID == id {
			return nil, errors.New("duplicate finance task")
		}
		if task.owner == owner {
			owned++
		}
	}
	if len(q.tasks) >= 32 || owned >= 2 {
		return nil, errors.New("finance queue full")
	}
	task := &financeTask{ID: id, State: "queued", CreatedAt: time.Now().Unix(), owner: owner, personal: personal, ready: make(chan struct{}), cancel: cancel}
	q.tasks = append(q.tasks, task)
	if len(q.tasks) == 1 {
		task.State = "running"
		close(task.ready)
	}
	return task, nil
}

func (q *financeQueue) finish(task *financeTask) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, candidate := range q.tasks {
		if candidate != task {
			continue
		}
		copy(q.tasks[i:], q.tasks[i+1:])
		q.tasks[len(q.tasks)-1] = nil
		q.tasks = q.tasks[:len(q.tasks)-1]
		if i == 0 && len(q.tasks) > 0 && q.tasks[0].State == "queued" {
			q.tasks[0].State = "running"
			close(q.tasks[0].ready)
		}
		return
	}
}

func (q *financeQueue) list(owner int, admin bool, id string) []financeTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := []financeTask{}
	for i, task := range q.tasks {
		if task.owner != owner || (!admin && !task.personal) || (id != "" && task.ID != id) {
			continue
		}
		result = append(result, financeTask{ID: task.ID, State: task.State, Ahead: i, CreatedAt: task.CreatedAt})
	}
	return result
}

func (q *financeQueue) cancelTask(owner int, id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, task := range q.tasks {
		if task.owner != owner || task.ID != id {
			continue
		}
		task.cancel()
		if task.State == "queued" {
			copy(q.tasks[i:], q.tasks[i+1:])
			q.tasks[len(q.tasks)-1] = nil
			q.tasks = q.tasks[:len(q.tasks)-1]
		} else {
			task.State = "cancelling"
		}
		return true
	}
	return false
}

// List and cancel use fresh identity checks too. Even administrators cannot
// inspect or cancel a different user's tasks through these endpoints.
func GetFinanceTasks(c *gin.Context) {
	user, ok := authorizeFinance(c, true)
	if !ok {
		return
	}
	common.ApiSuccess(c, channelFinanceQueue.list(user.Id, user.Role >= common.RoleAdminUser, c.Query("task_id")))
}

func CancelFinanceTask(c *gin.Context) {
	user, ok := authorizeFinance(c, true)
	if !ok {
		return
	}
	if c.Query("task_id") == "" {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	common.ApiSuccess(c, gin.H{"cancelled": channelFinanceQueue.cancelTask(user.Id, c.Query("task_id"))})
}
