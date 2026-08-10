package service

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const exportDir = "data/exports"

type logExportCenter struct {
	queue   chan int
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started atomic.Bool
}

var exportCenter *logExportCenter

// ExportQueueStats exposes runtime metrics for the admin dashboard.
type ExportQueueStats struct {
	QueueDepth        int   `json:"queue_depth"`
	QueueCapacity     int   `json:"queue_capacity"`
	WorkerCount       int   `json:"worker_count"`
	TotalStorageBytes int64 `json:"total_storage_bytes"`
	PendingCount      int64 `json:"pending_count"`
	ProcessingCount   int64 `json:"processing_count"`
	DoneCount         int64 `json:"done_count"`
	FailedCount       int64 `json:"failed_count"`
}

// InitLogExportWorker boots the background worker pool. Call once at startup.
func InitLogExportWorker() {
	cfg := operation_setting.GetLogExportSetting()

	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		common.SysError("log export: failed to create export dir: " + err.Error())
		return
	}

	ec := &logExportCenter{
		queue:  make(chan int, cfg.QueueSize),
		stopCh: make(chan struct{}),
	}

	for i := 0; i < cfg.WorkerCount; i++ {
		ec.wg.Add(1)
		go ec.runWorker(i)
	}

	// Crash recovery: re-enqueue pending/processing tasks from a previous
	// unclean shutdown so they don't stay stuck forever.
	tasks, err := model.GetPendingExportTasks()
	if err != nil {
		common.SysError("log export: failed to load pending tasks: " + err.Error())
	} else {
		for _, t := range tasks {
			select {
			case ec.queue <- t.Id:
			default:
				common.SysError(fmt.Sprintf("log export: queue full, cannot recover task %d", t.Id))
			}
		}
		if len(tasks) > 0 {
			common.SysLog(fmt.Sprintf("log export: recovered %d pending tasks", len(tasks)))
		}
	}

	go ec.cleanupLoop()

	ec.started.Store(true)
	exportCenter = ec
	common.SysLog(fmt.Sprintf("log export: started %d workers, queue capacity %d", cfg.WorkerCount, cfg.QueueSize))
}

// EnqueueExportTask pushes a task ID into the worker queue.
// Fire-and-forget: logs a warning if the queue is full.
func EnqueueExportTask(taskId int) {
	if exportCenter == nil || !exportCenter.started.Load() {
		common.SysError(fmt.Sprintf("log export: worker not started, cannot enqueue task %d", taskId))
		return
	}
	select {
	case exportCenter.queue <- taskId:
	default:
		common.SysError(fmt.Sprintf("log export: queue full, dropping task %d", taskId))
	}
}

// GetExportQueueStats returns live queue/storage metrics.
func GetExportQueueStats() *ExportQueueStats {
	stats := &ExportQueueStats{}
	if exportCenter == nil || !exportCenter.started.Load() {
		return stats
	}
	stats.QueueDepth = len(exportCenter.queue)
	stats.QueueCapacity = cap(exportCenter.queue)

	cfg := operation_setting.GetLogExportSetting()
	stats.WorkerCount = cfg.WorkerCount

	if total, err := model.GetTotalExportFileSize(); err == nil {
		stats.TotalStorageBytes = total
	}

	if cnt, err := model.GetExportTaskCountByStatus(model.ExportStatusPending); err == nil {
		stats.PendingCount = cnt
	}
	if cnt, err := model.GetExportTaskCountByStatus(model.ExportStatusProcessing); err == nil {
		stats.ProcessingCount = cnt
	}
	if cnt, err := model.GetExportTaskCountByStatus(model.ExportStatusDone); err == nil {
		stats.DoneCount = cnt
	}
	if cnt, err := model.GetExportTaskCountByStatus(model.ExportStatusFailed); err == nil {
		stats.FailedCount = cnt
	}

	return stats
}

// ---------------------------------------------------------------------------
// worker loop
// ---------------------------------------------------------------------------

func (ec *logExportCenter) runWorker(workerId int) {
	defer ec.wg.Done()
	for {
		select {
		case <-ec.stopCh:
			return
		case taskId, ok := <-ec.queue:
			if !ok {
				return
			}
			ec.processExportTask(taskId)
		}
	}
}

// processExportTask is the main work unit: load task, stream CSV, gzip, email.
func (ec *logExportCenter) processExportTask(taskId int) {
	task, err := model.GetLogExportTaskById(taskId)
	if err != nil {
		common.SysError(fmt.Sprintf("log export: task %d not found: %v", taskId, err))
		return
	}
	if task.Status != model.ExportStatusPending {
		return
	}

	now := common.GetTimestamp()
	_ = model.UpdateLogExportTask(taskId, map[string]interface{}{
		"status":       model.ExportStatusProcessing,
		"started_time": now,
	})

	rowCount, fileSize, exportErr := ec.generateExport(task)
	if exportErr != nil {
		_ = model.UpdateLogExportTask(taskId, map[string]interface{}{
			"status":         model.ExportStatusFailed,
			"error_message":  truncateStr(exportErr.Error(), 500),
			"completed_time": common.GetTimestamp(),
		})
		common.SysError(fmt.Sprintf("log export: task %d failed: %v", taskId, exportErr))
		return
	}

	gzPath := filepath.Join(exportDir, fmt.Sprintf("%d.csv.gz", taskId))
	_ = model.UpdateLogExportTask(taskId, map[string]interface{}{
		"status":         model.ExportStatusDone,
		"file_path":      gzPath,
		"file_size":      fileSize,
		"row_count":      rowCount,
		"progress":       100,
		"completed_time": common.GetTimestamp(),
	})

	ec.enforceStorageLimit()

	// Reload to get the fully updated record for the email.
	if updated, err := model.GetLogExportTaskById(taskId); err == nil {
		ec.sendExportCompleteEmail(updated)
	}
}

// generateExport streams logs into a temp CSV then gzip-compresses it.
// Returns (rowCount, compressedFileSize, error).
func (ec *logExportCenter) generateExport(task *model.LogExportTask) (int, int64, error) {
	cfg := operation_setting.GetLogExportSetting()
	maxBytes := int64(cfg.MaxFileSizeMB) * 1024 * 1024

	csvPath := filepath.Join(exportDir, fmt.Sprintf("%d.csv", task.Id))
	gzPath := filepath.Join(exportDir, fmt.Sprintf("%d.csv.gz", task.Id))

	csvFile, err := os.Create(csvPath)
	if err != nil {
		return 0, 0, fmt.Errorf("create csv: %w", err)
	}
	defer os.Remove(csvPath)
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	// BOM for Excel compatibility
	_, _ = csvFile.Write([]byte{0xEF, 0xBB, 0xBF})
	headers := []string{
		"时间", "类型", "令牌名称", "模型名称", "花费",
		"提示词tokens", "补全tokens", "请求耗时ms", "分组", "请求ID", "详情",
	}
	if err := writer.Write(headers); err != nil {
		return 0, 0, fmt.Errorf("write csv header: %w", err)
	}

	filters, err := parseExportFilters(task.Filters)
	if err != nil {
		return 0, 0, fmt.Errorf("parse filters: %w", err)
	}
	if err = validateExportFilters(filters); err != nil {
		return 0, 0, err
	}

	ctx := context.Background()
	var rowCount int
	var truncated bool

	streamErr := model.ExportUserLogs(ctx, task.UserId, filters.Type,
		filters.StartTimestamp, filters.EndTimestamp,
		filters.ModelName, filters.TokenName, filters.Group, "", "",
		func(logs []*model.Log) error {
			for _, log := range logs {
				if truncated {
					return nil
				}
				record := []string{
					time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
					model.LogTypeLabel(log.Type),
					log.TokenName,
					log.ModelName,
					exportFormatQuota(log.Quota),
					strconv.Itoa(log.PromptTokens),
					strconv.Itoa(log.CompletionTokens),
					strconv.Itoa(log.UseTime),
					log.Group,
					log.RequestId,
					log.Content,
				}
				if err := writer.Write(record); err != nil {
					return err
				}
				rowCount++

				// Every 1000 rows: flush, update progress, check size limit
				if rowCount%1000 == 0 {
					writer.Flush()
					_ = model.UpdateLogExportTask(task.Id, map[string]interface{}{
						"progress":  estimateProgress(rowCount),
						"row_count": rowCount,
					})
					if info, statErr := csvFile.Stat(); statErr == nil && info.Size() > maxBytes {
						truncated = true
						_ = model.UpdateLogExportTask(task.Id, map[string]interface{}{
							"error_message": fmt.Sprintf("file truncated at %d rows (exceeded %d MB limit)", rowCount, cfg.MaxFileSizeMB),
						})
						return nil
					}
				}
			}
			return nil
		})

	writer.Flush()
	if fErr := writer.Error(); fErr != nil {
		return rowCount, 0, fmt.Errorf("csv flush: %w", fErr)
	}
	if streamErr != nil {
		return rowCount, 0, fmt.Errorf("stream logs: %w", streamErr)
	}
	csvFile.Close()

	fileSize, err := compressFileGzip(csvPath, gzPath)
	if err != nil {
		return rowCount, 0, fmt.Errorf("gzip: %w", err)
	}
	return rowCount, fileSize, nil
}

// ---------------------------------------------------------------------------
// email notification (best-effort)
// ---------------------------------------------------------------------------

func (ec *logExportCenter) sendExportCompleteEmail(task *model.LogExportTask) {
	if task.Email == "" {
		return
	}

	cfg := operation_setting.GetLogExportSetting()
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	downloadURL := fmt.Sprintf("%s/api/log/self/export-download/%d", base, task.Id)
	expireTime := time.Unix(task.CompletedTime+int64(cfg.RetentionHours)*3600, 0).
		Format("2006-01-02 15:04:05")

	vars := map[string]string{
		"system_name":           html.EscapeString(common.SystemName),
		"site_name":             html.EscapeString(common.SystemName),
		"server_address":        html.EscapeString(base),
		"heading":               "数据导出完成",
		"intro":                 "你的日志导出已完成，请在有效期内下载。",
		"username":              html.EscapeString(task.Username),
		"row_count":             strconv.Itoa(task.RowCount),
		"file_size":             humanizeBytes(task.FileSize),
		"download_url":          downloadURL,
		"expire_time":           expireTime,
		"action_url":            downloadURL,
		"action_label":          "下载文件",
		"info_table":            buildExportInfoTable(task, expireTime),
		"content_preview_block": "",
	}

	subject, body := RenderEmailByKey(constant.EmailTemplateKeyLogExportComplete, vars)
	if subject == "" && body == "" {
		common.SysError("log export: email template not found: " + constant.EmailTemplateKeyLogExportComplete)
		return
	}
	if err := common.SendEmail(subject, task.Email, body); err != nil {
		common.SysError(fmt.Sprintf("log export: send email failed for task %d: %v", task.Id, err))
	}
}

func buildExportInfoTable(task *model.LogExportTask, expireTime string) string {
	rows := []struct{ Label, Value string }{
		{"导出行数", strconv.Itoa(task.RowCount)},
		{"文件大小", humanizeBytes(task.FileSize)},
		{"过期时间", expireTime},
	}
	var sb strings.Builder
	sb.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;">`)
	for _, r := range rows {
		sb.WriteString(fmt.Sprintf(
			`<tr><td style="padding:10px 0;color:#8e8e93;">%s</td>`+
				`<td style="padding:10px 0;color:#1d1d1f;text-align:right;">%s</td></tr>`,
			html.EscapeString(r.Label), html.EscapeString(r.Value),
		))
	}
	sb.WriteString(`</table>`)
	return sb.String()
}

// ---------------------------------------------------------------------------
// storage management
// ---------------------------------------------------------------------------

func (ec *logExportCenter) enforceStorageLimit() {
	cfg := operation_setting.GetLogExportSetting()
	maxBytes := int64(cfg.MaxTotalStorageMB) * 1024 * 1024

	total, err := model.GetTotalExportFileSize()
	if err != nil {
		common.SysError("log export: get total file size: " + err.Error())
		return
	}
	if total <= maxBytes {
		return
	}

	// Delete oldest completed files first (beforeTimestamp=0 returns all done tasks with files)
	tasks, err := model.GetCompletedExportTasksForCleanup(0)
	if err != nil {
		common.SysError("log export: cleanup query: " + err.Error())
		return
	}
	for _, t := range tasks {
		if total <= maxBytes {
			break
		}
		if t.FilePath != "" {
			_ = os.Remove(t.FilePath)
			total -= t.FileSize
			_ = model.UpdateLogExportTask(t.Id, map[string]interface{}{
				"file_path": "",
				"file_size": 0,
			})
		}
	}
}

func (ec *logExportCenter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ec.stopCh:
			return
		case <-ticker.C:
			ec.cleanupExpiredFiles()
		}
	}
}

func (ec *logExportCenter) cleanupExpiredFiles() {
	cfg := operation_setting.GetLogExportSetting()
	cutoff := common.GetTimestamp() - int64(cfg.RetentionHours)*3600
	tasks, err := model.GetCompletedExportTasksForCleanup(cutoff)
	if err != nil {
		common.SysError("log export: cleanup expired query: " + err.Error())
		return
	}
	for _, t := range tasks {
		if t.FilePath != "" {
			_ = os.Remove(t.FilePath)
			_ = model.UpdateLogExportTask(t.Id, map[string]interface{}{
				"file_path": "",
				"file_size": 0,
			})
		}
	}
	if len(tasks) > 0 {
		common.SysLog(fmt.Sprintf("log export: cleaned up %d expired files", len(tasks)))
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type exportFilters struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	Type           int    `json:"type"`
	ModelName      string `json:"model_name"`
	TokenName      string `json:"token_name"`
	Group          string `json:"group"`
}

func parseExportFilters(filtersJSON string) (*exportFilters, error) {
	f := &exportFilters{}
	if filtersJSON == "" {
		return f, nil
	}
	if err := common.UnmarshalJsonStr(filtersJSON, f); err != nil {
		return nil, err
	}
	return f, nil
}

func validateExportFilters(filters *exportFilters) error {
	if filters == nil || filters.StartTimestamp <= 0 || filters.EndTimestamp <= 0 || filters.StartTimestamp > filters.EndTimestamp {
		return errors.New("invalid export time range")
	}
	return nil
}

func compressFileGzip(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		return 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, err
	}
	if err := out.Sync(); err != nil {
		return 0, err
	}

	info, err := out.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), suffixes[exp])
}

func exportFormatQuota(quota int) string {
	dollars := float64(quota) / common.QuotaPerUnit
	return fmt.Sprintf("%.6f", dollars)
}

func estimateProgress(rowCount int) int {
	// Simple heuristic capped at 95% until completion is confirmed.
	p := rowCount / 100
	if p > 95 {
		p = 95
	}
	if p < 1 {
		p = 1
	}
	return p
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
