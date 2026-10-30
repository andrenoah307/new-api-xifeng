package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type channelCostReportHandler struct {
	aggregate func(context.Context, model.CostReportPolicy, int64, int64) ([]model.ChannelCostReportRow, error)
	skipEmpty func(context.Context, *model.ChannelCostReportJob, string) (int64, error)
}

func (channelCostReportHandler) Type() string { return model.SystemTaskTypeChannelCostReport }

func init() {
	RegisterSystemTaskHandler(channelCostReportHandler{aggregate: model.AggregateChannelCostWindow, skipEmpty: model.SkipChannelCostReportEmptyWindows})
}

func (handler channelCostReportHandler) Run(ctx context.Context, task *model.SystemTask, runner string) {
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := task.DecodePayload(&payload); err != nil {
		finishInvalidChannelCostReportTask(task, runner)
		return
	}
	job, err := model.GetChannelCostReportJob(ctx, payload.JobID)
	if err != nil || job.SystemTaskID != task.TaskID {
		finishInvalidChannelCostReportTask(task, runner)
		return
	}
	status := "failed"
	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		// Query cancellation has completed before releasing the system-task lease.
		latest, readErr := model.GetChannelCostReportJobState(cleanupCtx, job.ID)
		if readErr == nil && latest.SystemTaskID == task.TaskID {
			if latest.Control == "pause" {
				status = "paused"
			}
			if latest.Control == "cancel" {
				status = "cancelled"
			}
		}
		if finishErr := model.FinishChannelCostReportJob(cleanupCtx, job, runner, status); finishErr != nil {
			logChannelCostReportFailure(cleanupCtx, job.ID, "finalize", finishErr)
			status = "failed"
		}
		systemStatus := model.SystemTaskStatusSucceeded
		if status == "failed" {
			systemStatus = model.SystemTaskStatusFailed
		}
		if err := model.FinishSystemTaskWithContext(cleanupCtx, task.TaskID, runner, systemStatus, nil, ""); err != nil {
			logChannelCostReportFailure(cleanupCtx, job.ID, "release", err)
		}
	}()
	if err := model.BeginChannelCostReportJob(ctx, job, runner); err != nil {
		logChannelCostReportFailure(ctx, job.ID, "begin", err)
		return
	}
	var policy model.CostReportPolicy
	if err := common.UnmarshalJsonStr(job.PolicyJSON, &policy); err != nil {
		logChannelCostReportFailure(ctx, job.ID, "policy", err)
		return
	}
	seekEmpty := true
	for job.Cursor < policy.End {
		latest, err := model.GetChannelCostReportJob(ctx, job.ID)
		if err != nil || latest.Control != "" || latest.SystemTaskID != task.TaskID {
			if err != nil {
				logChannelCostReportFailure(ctx, job.ID, "read", err)
			}
			return
		}
		job = latest
		if err := model.ValidateChannelCostReportSource(ctx, job); err != nil {
			logChannelCostReportFailure(ctx, job.ID, "source", err)
			return
		}
		if err := model.AuthorizeChannelCostReportOwner(ctx, job.OwnerID); err != nil {
			logChannelCostReportFailure(ctx, job.ID, "authorize", err)
			return
		}
		end := min(job.Cursor+3600, policy.End)
		windowCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		stopped := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-windowCtx.Done():
					return
				case <-ticker.C:
					current, readErr := model.GetChannelCostReportJobState(windowCtx, job.ID)
					if readErr != nil || current.Control != "" || current.SystemTaskID != task.TaskID || current.Status != "running" {
						cancel()
						return
					}
					if model.AuthorizeChannelCostReportOwner(windowCtx, current.OwnerID) != nil {
						cancel()
						return
					}
				}
			}
		}()
		var rows []model.ChannelCostReportRow
		var queryErr error
		queryStage := "aggregate"
		skipped := job.Cursor
		if seekEmpty && handler.skipEmpty != nil {
			queryStage = "seek"
			skipped, queryErr = handler.skipEmpty(windowCtx, job, runner)
		}
		if queryErr == nil && skipped == job.Cursor {
			queryStage = "aggregate"
			rows, queryErr = handler.aggregate(windowCtx, policy, job.Cursor, end)
		}
		contextErr := windowCtx.Err()
		cancel()
		close(done)
		<-stopped
		if queryErr != nil || contextErr != nil {
			if queryErr != nil {
				logChannelCostReportFailure(ctx, job.ID, queryStage, queryErr)
			} else {
				logChannelCostReportFailure(ctx, job.ID, queryStage, contextErr)
			}
			return
		}
		if skipped > job.Cursor {
			job.Cursor = skipped
			seekEmpty = false
			continue
		}
		if err := model.PublishChannelCostReportWindow(ctx, job, runner, end, rows); err != nil {
			logChannelCostReportFailure(ctx, job.ID, "publish", err)
			return
		}
		job.Cursor = end
		seekEmpty = len(rows) == 0
	}
	status = "completed"
}

func logChannelCostReportFailure(ctx context.Context, jobID, stage string, err error) {
	logger.LogWarn(ctx, fmt.Sprintf("channel cost report failed: job=%s stage=%s err=%v", jobID, stage, err))
}

func finishInvalidChannelCostReportTask(task *model.SystemTask, runner string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := model.FinishSystemTaskWithContext(ctx, task.TaskID, runner, model.SystemTaskStatusFailed, nil, "Invalid cost report task"); err != nil {
		logChannelCostReportFailure(ctx, task.TaskID, "invalid_task", err)
	}
}
