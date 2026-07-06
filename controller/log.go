package controller

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	if startTimestamp == 0 && requestId == "" && upstreamRequestId == "" {
		startTimestamp = time.Now().AddDate(0, 0, -30).Unix()
	}
	cachedTotal, _ := strconv.ParseInt(c.Query("total_count"), 10, 64)
	if cachedTotal < 0 || cachedTotal > 1000000 {
		cachedTotal = 0
	}
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId, cachedTotal)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	if startTimestamp == 0 && requestId == "" && upstreamRequestId == "" {
		startTimestamp = time.Now().AddDate(0, 0, -30).Unix()
	}
	cachedTotal, _ := strconv.ParseInt(c.Query("total_count"), 10, 64)
	if cachedTotal < 0 || cachedTotal > 1000000 {
		cachedTotal = 0
	}
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId, cachedTotal)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 {
		startTimestamp = time.Now().AddDate(0, 0, -30).Unix()
	}
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(0, logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 {
		startTimestamp = time.Now().AddDate(0, 0, -30).Unix()
	}
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(userId, logType, startTimestamp, endTimestamp, modelName, "", tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

// DeleteHistoryLogs is the legacy synchronous log cleanup endpoint (DELETE /api/log/).
// It deletes directly instead of going through the async system task. It is kept only
// for the classic frontend; the default frontend uses POST /api/system-task/log-cleanup.
// TODO: remove this handler (and its route) once the classic frontend is removed.
func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}

func formatQuotaAsDollar(quota int) string {
	dollars := float64(quota) / common.QuotaPerUnit
	return fmt.Sprintf("%.6f", dollars)
}

func ExportAllLogsCsv(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "导出必须指定起止时间"})
		return
	}
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")

	headers := []string{"时间", "类型", "用户名", "令牌名称", "模型名称", "花费", "提示词tokens", "补全tokens", "请求耗时ms", "渠道ID", "渠道名称", "分组", "请求ID", "IP", "详情"}
	exportCsvWithHeartbeat(c, headers, func(ctx context.Context, writer *csv.Writer) error {
		return model.ExportAllLogs(ctx, logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId, func(logs []*model.Log) error {
			for _, log := range logs {
				record := []string{
					time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
					model.LogTypeLabel(log.Type),
					log.Username,
					log.TokenName,
					log.ModelName,
					formatQuotaAsDollar(log.Quota),
					strconv.Itoa(log.PromptTokens),
					strconv.Itoa(log.CompletionTokens),
					strconv.Itoa(log.UseTime),
					strconv.Itoa(log.ChannelId),
					log.ChannelName,
					log.Group,
					log.RequestId,
					log.Ip,
					log.Content,
				}
				if err := writer.Write(record); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func ExportUserLogsCsv(c *gin.Context) {
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "导出必须指定起止时间"})
		return
	}
	if endTimestamp-startTimestamp > 31*86400 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "导出时间范围不能超过一个月"})
		return
	}
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")

	headers := []string{"时间", "类型", "令牌名称", "模型名称", "花费", "提示词tokens", "补全tokens", "请求耗时ms", "分组", "请求ID", "详情"}
	exportCsvWithHeartbeat(c, headers, func(ctx context.Context, writer *csv.Writer) error {
		return model.ExportUserLogs(ctx, userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId, func(logs []*model.Log) error {
			for _, log := range logs {
				record := []string{
					time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
					model.LogTypeLabel(log.Type),
					log.TokenName,
					log.ModelName,
					formatQuotaAsDollar(log.Quota),
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
			}
			return nil
		})
	})
}

func exportCsvWithHeartbeat(c *gin.Context, headers []string, writeFn func(ctx context.Context, writer *csv.Writer) error) {
	filename := fmt.Sprintf("logs_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-cache")

	writer := csv.NewWriter(c.Writer)
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer.Write(headers)
	writer.Flush()
	c.Writer.Flush()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	var mu sync.Mutex
	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				if f, ok := c.Writer.(http.Flusher); ok {
					f.Flush()
				}
				mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	err := writeFn(ctx, writer)
	cancel()

	mu.Lock()
	writer.Flush()
	c.Writer.Flush()
	mu.Unlock()

	if err != nil && ctx.Err() == nil {
		common.SysError("export logs failed: " + err.Error())
	}
}
