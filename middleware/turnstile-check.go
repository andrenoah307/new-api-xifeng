package middleware

import (
	"github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-gonic/gin"
)

// Preserve the upstream routing hook while applying per-request protection.
func TurnstileCheck() gin.HandlerFunc {
	handlers := map[string]gin.HandlerFunc{
		"/api/verification":   CaptchaCheck("email_verification"),
		"/api/reset_password": CaptchaCheck("password_reset_email"),
		"/api/user/login":     CaptchaCheck("login"),
		"/api/user/register":  CaptchaCheck("register"),
		"/api/user/checkin":   CaptchaCheck("checkin"),
	}
	return func(c *gin.Context) {
		handler, ok := handlers[c.FullPath()]
		if !ok {
			CaptchaFailure(c, captcha.ErrInvalid)
			return
		}
		handler(c)
	}
}
