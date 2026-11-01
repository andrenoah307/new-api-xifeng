package controller

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RevenueAutomation(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	row, config, err := model.GetRevenueAutomation(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	runs, err := model.ListRevenueAutomationRuns(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !authorizeLogRoot(c) {
		return
	}
	common.ApiSuccess(c, gin.H{"config": config, "revision": row.Revision, "next_at": row.NextAt, "runs": runs})
}

func SaveRevenueAutomation(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var request struct {
		Revision int64                         `json:"revision"`
		Config   model.RevenueAutomationConfig `json:"config"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, _, err := service.RevenueMailPreview(request.Config); err != nil {
		common.ApiError(c, err)
		return
	}
	err := model.SaveRevenueAutomation(c.Request.Context(), c.GetInt("id"), request.Revision, request.Config)
	if errors.Is(err, model.ErrRevenueAutomationConflict) || errors.Is(err, model.ErrCostReportConfigChanged) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func PreviewRevenueAutomationMail(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	if c.Param("action") != "preview" && c.Param("action") != "test" {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
	var config model.RevenueAutomationConfig
	if err := common.DecodeJson(c.Request.Body, &config); err != nil {
		common.ApiError(c, err)
		return
	}
	subject, body, err := service.RevenueMailPreview(config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.Param("action") == "test" {
		user, err := model.GetUserById(c.GetInt("id"), false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if !authorizeLogRoot(c) {
			return
		}
		state, err := common.SendRevenueEmail(c.Request.Context(), "[TEST] "+subject, user.Email, body)
		if err != nil {
			common.ApiError(c, errors.New("Test email failed: "+state))
			return
		}
	}
	common.ApiSuccess(c, gin.H{"subject": subject, "body": body})
}

func ControlRevenueAutomationRun(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	config, settings, err := model.ClaimRevenueAutomationControl(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if config == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Revenue automation changed or is busy"})
		return
	}
	defer model.ReleaseRevenueAutomation(context.Background(), config)
	var run model.RevenueAutomationRun
	if err = model.DB.WithContext(c.Request.Context()).First(&run, "id = ?", c.Param("id")).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if (run.Revision != config.Revision && c.Param("action") != "dismiss") || (run.Status != "failed" && run.Status != "unknown") {
		common.ApiError(c, model.ErrRevenueAutomationConflict)
		return
	}
	var data service.RevenueAutomationData
	if err = common.UnmarshalJsonStr(run.Data, &data); err != nil {
		common.ApiError(c, err)
		return
	}
	switch c.Param("action") {
	case "dismiss":
		run.Status = "cancelled"
	case "retry":
		if !settings.Enabled {
			common.ApiError(c, errors.New("Enable automation before retrying"))
			return
		}
		for _, delivery := range data.Deliveries {
			if (delivery.Status == "sending" || delivery.Status == "unknown") && c.Query("confirm_duplicate_risk") != "true" {
				common.ApiError(c, errors.New("Confirm possible duplicate delivery"))
				return
			}
		}
		if run.Status == "unknown" && c.Query("confirm_duplicate_risk") != "true" {
			common.ApiError(c, errors.New("Confirm possible duplicate delivery"))
			return
		}
		for i := range data.Deliveries {
			if data.Deliveries[i].Status != "accepted" {
				data.Deliveries[i].Status = "pending"
				data.Deliveries[i].Attempts = 0
				data.Deliveries[i].NextAt = 0
			}
		}
		run.Status = "waiting"
		run.Error = ""
	default:
		c.Status(http.StatusNotFound)
		return
	}
	encoded, err := common.Marshal(data)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	run.Data = string(encoded)
	run.LastActorID = c.GetInt("id")
	if err = model.SaveRevenueAutomationRun(c.Request.Context(), config, &run); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func RevenueAutomationRunDetail(c *gin.Context) {
	if !authorizeLogRoot(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var run model.RevenueAutomationRun
	if err := model.DB.WithContext(c.Request.Context()).First(&run, "id = ?", c.Param("id")).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var data service.RevenueAutomationData
	if err := common.UnmarshalJsonStr(run.Data, &data); err != nil {
		common.ApiError(c, err)
		return
	}
	if !authorizeLogRoot(c) {
		return
	}
	common.ApiSuccess(c, gin.H{"run": run, "data": data})
}
