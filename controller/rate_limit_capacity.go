package controller

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var rateLimitCapacityService = service.DefaultRateLimitCapacityService()

// GetRateLimitCapacity is a read-only user endpoint. It reads the group through
// the same Redis-backed path as /api/user/groups, so both endpoints share the
// current group without requiring a users-table query on a cache hit. The
// service always returns a body with HTTP 200, including backend degradation
// details.
func GetRateLimitCapacity(c *gin.Context) {
	if rateLimitCapacityService == nil {
		rateLimitCapacityService = service.DefaultRateLimitCapacityService()
	}
	userGroup, _ := model.GetUserGroup(c.GetInt("id"), false)
	requestContext := context.Background()
	if c.Request != nil {
		requestContext = c.Request.Context()
	}
	response := rateLimitCapacityService.Get(requestContext, service.CapacityRequest{
		UserID:    c.GetInt("id"),
		UserGroup: userGroup,
		Country:   requestip.GetClientCountry(c),
		IsAdmin:   c.GetInt("role") >= common.RoleAdminUser,
		Scope:     c.DefaultQuery("scope", "top"),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}
