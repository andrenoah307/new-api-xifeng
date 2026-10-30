package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type channelCostReportJobDTO struct {
	*model.ChannelCostReportJob
	Start     int64                  `json:"start"`
	End       int64                  `json:"end"`
	ChannelID int                    `json:"channel_id"`
	Progress  int                    `json:"progress"`
	Policy    model.CostReportPolicy `json:"policy"`
}

func channelCostReportJobResponse(job *model.ChannelCostReportJob) (channelCostReportJobDTO, error) {
	result := channelCostReportJobDTO{ChannelCostReportJob: job}
	if err := common.UnmarshalJsonStr(job.PolicyJSON, &result.Policy); err != nil {
		return result, err
	}
	result.Start, result.End, result.ChannelID = result.Policy.Start, result.Policy.End, result.Policy.ChannelID
	if job.TotalWindows > 0 {
		result.Progress = job.ProcessedWindows * 100 / job.TotalWindows
	}
	return result, nil
}

func ListChannelCostReportJobs(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	var jobs []*model.ChannelCostReportJob
	var total int64
	var err error
	page, pageSize := 1, 10
	paginated := c.Request.URL.Query().Has("page")
	if paginated {
		page, err = strconv.Atoi(c.Query("page"))
		if err == nil {
			pageSize, err = strconv.Atoi(c.DefaultQuery("page_size", "10"))
		}
		keyword, status := strings.TrimSpace(c.Query("keyword")), c.Query("status")
		validStatus := status == "" || status == "pending" || status == "running" || status == "paused" || status == "completed" || status == "failed" || status == "cancelled"
		if err != nil || page < 1 || page > 1000000 || pageSize < 1 || pageSize > 100 || len(keyword) > 128 || !validStatus {
			logRootUnavailable(c, i18n.MsgInvalidParams)
			return
		}
		jobs, total, err = model.ListChannelCostReportJobsPage(c.Request.Context(), page, pageSize, keyword, status)
	} else {
		jobs, err = model.ListChannelCostReportJobs(c.Request.Context(), 50)
	}
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	data := make([]channelCostReportJobDTO, 0, len(jobs))
	for _, job := range jobs {
		item, err := channelCostReportJobResponse(job)
		if err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
			return
		}
		data = append(data, item)
	}
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	if paginated {
		common.ApiSuccess(c, gin.H{"items": data, "total": total, "page": page, "page_size": pageSize})
		return
	}
	common.ApiSuccess(c, data)
}

func CreateChannelCostReportJob(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	var request struct {
		RequestKey   string                 `json:"request_key"`
		Start        int64                  `json:"start"`
		End          int64                  `json:"end"`
		ChannelID    int                    `json:"channel_id"`
		Rates        []model.CostReportRate `json:"rates"`
		FallbackMode string                 `json:"fallback_mode"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	if common.DecodeJson(c.Request.Body, &request) != nil || request.End > time.Now().Unix() {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	policy := model.CostReportPolicy{Start: request.Start, End: request.End, ChannelID: request.ChannelID,
		QuotaPerUnit: decimal.NewFromFloat(common.QuotaPerUnit).String(), Rates: request.Rates, FallbackMode: request.FallbackMode}
	if err := model.ValidateCostReportPolicy(policy); err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	job, err := model.CreateChannelCostReportJob(c.Request.Context(), c.GetInt("id"), request.RequestKey, policy)
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	data, err := channelCostReportJobResponse(job)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, data)
}

func GetChannelCostReportJob(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	job, err := model.GetChannelCostReportJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	data, err := channelCostReportJobResponse(job)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, data)
}

func ControlChannelCostReportJob(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	if err := model.ControlChannelCostReportJob(c.Request.Context(), c.Param("id"), c.Param("action")); err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	GetChannelCostReportJob(c)
}

type channelCostReportRowDTO struct {
	model.ChannelCostReportRow
	ChannelName  string  `json:"channel_name"`
	Cost         string  `json:"cost"`
	NetAmount    string  `json:"net_amount"`
	KnownProfit  string  `json:"known_profit"`
	Profit       *string `json:"profit"`
	ProfitMargin *string `json:"profit_margin"`
	CostRatio    *string `json:"cost_ratio"`
	Coverage     string  `json:"coverage"`
}

func projectChannelCostReportRow(row model.ChannelCostReportRow) (channelCostReportRowDTO, error) {
	result := channelCostReportRowDTO{ChannelCostReportRow: row}
	amounts := make([]decimal.Decimal, 5)
	for index, text := range []string{row.ConsumeAmount, row.RefundAmount, row.OriginalCost, row.FallbackCost, row.KnownRevenue} {
		value, err := decimal.NewFromString(text)
		if err != nil || value.IsNegative() {
			return result, errors.New("invalid cost report amount")
		}
		amounts[index] = value
	}
	cost := amounts[2].Add(amounts[3])
	net := amounts[0].Sub(amounts[1])
	knownProfit := amounts[4].Sub(cost)
	result.Cost, result.NetAmount, result.KnownProfit = cost.String(), net.String(), knownProfit.String()
	result.Coverage = "1"
	if row.ConsumeCount > 0 {
		result.Coverage = decimal.NewFromInt(row.ConsumeCount-row.MissingCount).DivRound(decimal.NewFromInt(row.ConsumeCount), 18).String()
	}
	if amounts[4].IsPositive() {
		ratio := cost.DivRound(amounts[4], 18).String()
		result.CostRatio = &ratio
	}
	if row.MissingCount == 0 && row.RefundCount == 0 {
		profit := net.Sub(cost)
		value := profit.String()
		result.Profit = &value
		if amounts[0].IsPositive() {
			margin := profit.DivRound(amounts[0], 18).String()
			result.ProfitMargin = &margin
		}
	}
	return result, nil
}

func GetChannelCostReport(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	job, err := model.GetChannelCostReportJob(c.Request.Context(), c.Param("id"))
	if err != nil {
		logRootUnavailable(c, i18n.MsgInvalidParams)
		return
	}
	jobDTO, err := channelCostReportJobResponse(job)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	var stored []model.ChannelCostReportRow
	if common.UnmarshalJsonStr(job.RowsJSON, &stored) != nil || len(stored) > 5000 {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	rows, err := model.MergeChannelCostReportRows(nil, stored)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ChannelID)
	}
	names := map[int]string{}
	if len(ids) > 0 {
		var channels []struct {
			Id   int
			Name string
		}
		if err := model.DB.WithContext(c.Request.Context()).Model(&model.Channel{}).Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
			return
		}
		for _, channel := range channels {
			names[channel.Id] = channel.Name
		}
	}
	data := make([]channelCostReportRowDTO, 0, len(rows))
	for i, row := range rows {
		item, err := projectChannelCostReportRow(row)
		if err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
			return
		}
		item.ChannelName = names[row.ChannelID]
		if job.Status != "completed" {
			item.Profit, item.ProfitMargin = nil, nil
		}
		data = append(data, item)
		rows[i].ChannelID = 0
	}
	// Reuse the exact decimal reducer for totals; never add displayed floats.
	totals, err := model.MergeChannelCostReportRows(nil, rows)
	if err != nil {
		logRootUnavailable(c, i18n.MsgDatabaseError)
		return
	}
	totalRow := model.ChannelCostReportRow{ConsumeAmount: "0", RefundAmount: "0", OriginalCost: "0", FallbackCost: "0", KnownRevenue: "0", UnknownRevenue: "0"}
	if len(totals) > 0 {
		totalRow = totals[0]
	}
	total, err := projectChannelCostReportRow(totalRow)
	if err != nil || !authorizeLogRoot(c) {
		if err != nil {
			logRootUnavailable(c, i18n.MsgDatabaseError)
		}
		return
	}
	if job.Status != "completed" {
		total.Profit, total.ProfitMargin = nil, nil
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"job": jobDTO, "channels": data, "total": total})
}
