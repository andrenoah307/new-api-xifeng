package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerFinanceRoutes(api *gin.RouterGroup) {
	finance := api.Group("/finance", middleware.UserAuth(), middleware.FinanceModuleRequired())
	finance.GET("/records", middleware.RequirePermission(authz.FinanceRead), controller.GetFinanceRecords)
	finance.GET("/export", middleware.RequirePermission(authz.FinanceRead), middleware.RequirePermission(authz.FinanceExport), middleware.CriticalRateLimit(), controller.ExportFinanceRecords)
	finance.GET("/policy", middleware.RequirePermission(authz.FinanceRead), controller.GetFinancePolicy)
	finance.GET("/policy/choices", middleware.AdminAuth(), controller.GetFinancePolicyChoices)
	finance.PUT("/policy", middleware.AdminAuth(), controller.UpdateFinancePolicy)
}
