package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerCaptchaRoutes(api *gin.RouterGroup) {
	api.POST("/captcha/verify", middleware.CaptchaVerificationRateLimit(), controller.VerifyCaptcha)
	api.GET("/captcha/settings", middleware.RootAuth(), controller.CaptchaSettings)
	api.PUT("/captcha/settings", middleware.RootAuth(), controller.CaptchaSettings)
}
