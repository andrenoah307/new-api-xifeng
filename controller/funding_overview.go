package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetFundingOverview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetInt("role") != common.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	// A previously issued root session must not outlive a role/status change.
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil || user.Role != common.RoleRootUser || user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	start, startErr := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, endErr := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if startErr != nil || endErr != nil || !model.ValidFundingOverviewRange(start, end, time.Now().Unix()) {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	data, err := service.GetFundingOverview(c.Request.Context(), start, end)
	if err != nil {
		common.SysError("funding overview query failed")
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
