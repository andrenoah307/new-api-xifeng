package controller

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type offlineExportFilters struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	ModelName      string `json:"model_name"`
	TokenName      string `json:"token_name"`
	ChannelId      int    `json:"channel_id"`
}

type submitOfflineExportRequest struct {
	Filters offlineExportFilters `json:"filters"`
	Email   string               `json:"email"`
}

// SubmitOfflineExport creates an offline log export task for the current user.
func SubmitOfflineExport(c *gin.Context) {
	if !common.LogExportOfflineEnabled {
		common.ApiErrorMsg(c, "离线导出功能未启用")
		return
	}

	var req submitOfflineExportRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数")
		return
	}
	if req.Email == "" {
		common.ApiErrorMsg(c, "邮箱地址不能为空")
		return
	}

	userId := c.GetInt("id")
	username := c.GetString("username")

	// Cooldown check
	cfg := operation_setting.GetLogExportSetting()
	sinceTimestamp := time.Now().Unix() - int64(cfg.UserCooldownHours*3600)
	recentCount, err := model.GetUserRecentExportCount(userId, sinceTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if recentCount > 0 {
		nextAvailable := time.Now().Add(time.Duration(cfg.UserCooldownHours) * time.Hour)
		c.JSON(http.StatusOK, gin.H{
			"success":             false,
			"message":             fmt.Sprintf("导出冷却中，请在 %d 小时后重试", cfg.UserCooldownHours),
			"next_available_time": nextAvailable.Unix(),
		})
		return
	}

	filtersJSON, err := common.Marshal(req.Filters)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	task := &model.LogExportTask{
		UserId:   userId,
		Username: username,
		Email:    req.Email,
		Filters:  string(filtersJSON),
		Status:   model.ExportStatusPending,
	}
	if err = model.CreateLogExportTask(task); err != nil {
		common.ApiError(c, err)
		return
	}

	service.EnqueueExportTask(task.Id)
	common.ApiSuccess(c, gin.H{"id": task.Id})
}

// GetUserExportTaskList returns the current user's export tasks with pagination.
func GetUserExportTaskList(c *gin.Context) {
	userId := c.GetInt("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	tasks, total, err := model.GetUserExportTasks(userId, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": tasks,
		"total": total,
	})
}

// DownloadExportFile serves the completed export file for the owning user.
func DownloadExportFile(c *gin.Context) {
	taskId, err := strconv.Atoi(c.Param("id"))
	if err != nil || taskId <= 0 {
		common.ApiErrorMsg(c, "无效的任务 ID")
		return
	}

	task, err := model.GetLogExportTaskById(taskId)
	if err != nil {
		// Return 404 regardless of reason to avoid leaking task existence
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "任务不存在"})
		return
	}

	// Security: only the owning user can download; return 404 (not 403)
	if task.UserId != c.GetInt("id") {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "任务不存在"})
		return
	}

	serveExportFile(c, task)
}

// GetAdminExportTasks returns all users' export tasks for admin review.
func GetAdminExportTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}

	tasks, total, err := model.GetAllExportTasks(page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": tasks,
		"total": total,
	})
}

// GetExportQueueStats returns the worker queue statistics.
func GetExportQueueStats(c *gin.Context) {
	stats := service.GetExportQueueStats()
	common.ApiSuccess(c, stats)
}

// GetExportConfig returns the current log export configuration.
func GetExportConfig(c *gin.Context) {
	cfg := operation_setting.GetLogExportSetting()
	common.ApiSuccess(c, cfg)
}

// UpdateExportConfig persists the log export configuration.
func UpdateExportConfig(c *gin.Context) {
	var req operation_setting.LogExportSetting
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}

	operation_setting.NormalizeLogExportSetting(&req)

	configMap, err := config.ConfigToMap(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for key, value := range configMap {
		if err = model.UpdateOption("log_export."+key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// Re-read to return the normalized result
	GetExportConfig(c)
}

// CancelExportTask cancels a pending or processing export task (admin only).
func CancelExportTask(c *gin.Context) {
	taskId, err := strconv.Atoi(c.Param("id"))
	if err != nil || taskId <= 0 {
		common.ApiErrorMsg(c, "无效的任务 ID")
		return
	}

	task, err := model.GetLogExportTaskById(taskId)
	if err != nil {
		common.ApiErrorMsg(c, "任务不存在")
		return
	}

	if task.Status != model.ExportStatusPending && task.Status != model.ExportStatusProcessing {
		common.ApiErrorMsg(c, "仅待处理或处理中的任务可取消")
		return
	}

	if err = model.UpdateLogExportTask(taskId, map[string]interface{}{
		"status": model.ExportStatusCancelled,
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": taskId})
}

// AdminDownloadExportFile serves the completed export file for admin (no ownership check).
func AdminDownloadExportFile(c *gin.Context) {
	taskId, err := strconv.Atoi(c.Param("id"))
	if err != nil || taskId <= 0 {
		common.ApiErrorMsg(c, "无效的任务 ID")
		return
	}

	task, err := model.GetLogExportTaskById(taskId)
	if err != nil {
		common.ApiErrorMsg(c, "任务不存在")
		return
	}

	serveExportFile(c, task)
}

// serveExportFile validates the task state and sends the file to the client.
func serveExportFile(c *gin.Context, task *model.LogExportTask) {
	if task.Status != model.ExportStatusDone || task.FilePath == "" {
		common.ApiErrorMsg(c, "导出文件尚未准备好")
		return
	}
	if _, err := os.Stat(task.FilePath); os.IsNotExist(err) {
		common.ApiErrorMsg(c, "导出文件已过期或已被清理")
		return
	}

	filename := filepath.Base(task.FilePath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(task.FilePath)
}
