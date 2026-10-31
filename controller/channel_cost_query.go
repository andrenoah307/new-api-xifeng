package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func QueryChannelCostReports(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	start, e1 := strconv.ParseInt(c.Query("start"), 10, 64)
	end, e2 := strconv.ParseInt(c.Query("end"), 10, 64)
	channel, e3 := strconv.Atoi(c.DefaultQuery("channel_id", "0"))
	if e1 != nil || e2 != nil || e3 != nil || start < 1 || end <= start || end > time.Now().Unix() || channel < 0 {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	result, err := model.QueryChannelCostReports(ctx, start, end, channel)
	if errors.Is(err, model.ErrCostQueryTooLarge) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "Too many summaries. Narrow the time range."})
		return
	}
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	budget, err := model.GetCostCheckpointBudget(ctx)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	rows := make([]channelCostReportRowDTO, 0, len(result.Rows))
	var total *channelCostReportRowDTO
	if result.Complete {
		ids := make([]int, 0, len(result.Rows))
		for _, row := range result.Rows {
			ids = append(ids, row.ChannelID)
		}
		names := map[int]string{}
		if len(ids) > 0 {
			var channels []struct {
				Id   int
				Name string
			}
			if err := model.DB.WithContext(ctx).Model(&model.Channel{}).Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
				logRootUnavailable(c, i18n.MsgDatabaseError)
				return
			}
			for _, item := range channels {
				names[item.Id] = item.Name
			}
		}
		for i, row := range result.Rows {
			item, err := projectChannelCostReportRow(row)
			if err != nil {
				logRootUnavailable(c, i18n.MsgDatabaseError)
				return
			}
			item.ChannelName = names[row.ChannelID]
			rows = append(rows, item)
			result.Rows[i].ChannelID = 0
		}
		merged, err := model.MergeChannelCostReportRows(nil, result.Rows)
		if err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
			return
		}
		row := model.ChannelCostReportRow{ConsumeAmount: "0", RefundAmount: "0", OriginalCost: "0", FallbackCost: "0", KnownRevenue: "0", UnknownRevenue: "0"}
		if len(merged) > 0 {
			row = merged[0]
		}
		projected, err := projectChannelCostReportRow(row)
		if err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
			return
		}
		total = &projected
	}
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"start": start, "end": end, "channel_id": channel, "complete": result.Complete, "missing": result.Missing, "sources": result.Sources, "policy": result.Policy, "channels": rows, "total": total, "storage": budget, "queried_at": time.Now().Unix()})
}
