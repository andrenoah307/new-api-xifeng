package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
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
	cachedTotal, _ := strconv.ParseInt(c.Query("total_count"), 10, 64)
	if cachedTotal < 0 || cachedTotal > 1000000 {
		cachedTotal = 0
	}
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, cachedTotal)
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
	cachedTotal, _ := strconv.ParseInt(c.Query("total_count"), 10, 64)
	if cachedTotal < 0 || cachedTotal > 1000000 {
		cachedTotal = 0
	}
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, cachedTotal)
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
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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
	username := c.GetString("username")
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
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
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

func ExportAllLogsCsv(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出必须指定起止时间"})
		return
	}
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")

	filename := fmt.Sprintf("logs_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Transfer-Encoding", "chunked")

	writer := csv.NewWriter(c.Writer)
	// BOM for Excel UTF-8 compatibility
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer.Write([]string{"时间", "类型", "用户名", "令牌名称", "模型名称", "消耗额度", "提示词tokens", "补全tokens", "请求耗时ms", "渠道ID", "渠道名称", "分组", "请求ID", "IP"})
	writer.Flush()

	err := model.ExportAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId, func(logs []*model.Log) error {
		for _, log := range logs {
			record := []string{
				time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
				model.LogTypeLabel(log.Type),
				log.Username,
				log.TokenName,
				log.ModelName,
				strconv.Itoa(log.Quota),
				strconv.Itoa(log.PromptTokens),
				strconv.Itoa(log.CompletionTokens),
				strconv.Itoa(log.UseTime),
				strconv.Itoa(log.ChannelId),
				log.ChannelName,
				log.Group,
				log.RequestId,
				log.Ip,
			}
			if err := writer.Write(record); err != nil {
				return err
			}
		}
		writer.Flush()
		c.Writer.Flush()
		return writer.Error()
	})
	if err != nil {
		common.SysError("export all logs failed: " + err.Error())
	}
}

func ExportUserLogsCsv(c *gin.Context) {
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startTimestamp == 0 || endTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出必须指定起止时间"})
		return
	}
	if endTimestamp-startTimestamp > 31*86400 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "导出时间范围不能超过一个月"})
		return
	}
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")

	filename := fmt.Sprintf("logs_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Transfer-Encoding", "chunked")

	writer := csv.NewWriter(c.Writer)
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer.Write([]string{"时间", "类型", "令牌名称", "模型名称", "消耗额度", "提示词tokens", "补全tokens", "请求耗时ms", "分组", "请求ID"})
	writer.Flush()

	err := model.ExportUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId, func(logs []*model.Log) error {
		for _, log := range logs {
			record := []string{
				time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05"),
				model.LogTypeLabel(log.Type),
				log.TokenName,
				log.ModelName,
				strconv.Itoa(log.Quota),
				strconv.Itoa(log.PromptTokens),
				strconv.Itoa(log.CompletionTokens),
				strconv.Itoa(log.UseTime),
				log.Group,
				log.RequestId,
			}
			if err := writer.Write(record); err != nil {
				return err
			}
		}
		writer.Flush()
		c.Writer.Flush()
		return writer.Error()
	})
	if err != nil {
		common.SysError("export user logs failed: " + err.Error())
	}
}
