package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// BotSystemTokenAuth never consults session cookies or relay API tokens.
func BotSystemTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid)})
			return
		}
		user, err := model.ValidateAccessToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgDatabaseError)})
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthAccessTokenInvalid)})
			return
		}
		id, idErr := strconv.Atoi(c.GetHeader("New-Api-User"))
		if idErr != nil || id != user.Id || user.Status != common.UserStatusEnabled || user.Role != common.RoleBotUser || !authz.Can(user.Id, user.Role, authz.LogsReadRedacted) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
			return
		}
		c.Set("id", user.Id)
		c.Set("role", user.Role)
		c.Set("use_access_token", true)
		c.Set("bot_system_token", parts[1])
		c.Next()
	}
}
