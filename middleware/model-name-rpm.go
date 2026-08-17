package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	realtimemetrics "github.com/QuantumNous/new-api/pkg/realtime_metrics"
	"github.com/QuantumNous/new-api/service/model_name_limiter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	// Kept for package-local compatibility; actual construction is delegated to
	// model_name_limiter so admission and inspection cannot drift.
	modelNameRPMModelKeyPrefix = "mdrl:v1:rpm:model:"
	modelNameRPMGroupKeyPrefix = "mdrl:v1:rpm:group:"
	modelNameRPMRetryAfter     = "60"
)

// This indirection keeps the gate's no-rule path directly testable without
// changing the limiter package contract. Production always points at the T2
// Acquire implementation.
var modelNameRPMAcquire = model_name_limiter.Acquire

type modelNameRPMResponseMode uint8

const (
	modelNameRPMOpenAIResponse modelNameRPMResponseMode = iota
	modelNameRPMTaskResponse
)

// enforceModelNameRPM returns true when the request may continue. On a normal
// policy rejection it writes the OpenAI-compatible 503 response and aborts the
// Gin context before returning false.
func enforceModelNameRPM(c *gin.Context, modelName, policyGroup, route string) bool {
	return enforceModelNameRPMWithResponse(c, modelName, policyGroup, route, modelNameRPMOpenAIResponse)
}

// EnforceModelNameRPMForTask is the task-protocol entry point. It shares the
// same admission and idempotency state as enforceModelNameRPM, but emits a
// dto.TaskError so task clients receive their native response shape.
func EnforceModelNameRPMForTask(c *gin.Context, modelName, policyGroup, route string) bool {
	return enforceModelNameRPMWithResponse(c, modelName, policyGroup, route, modelNameRPMTaskResponse)
}

func enforceModelNameRPMWithResponse(c *gin.Context, modelName, policyGroup, route string, responseMode modelNameRPMResponseMode) bool {
	if _, checked := c.Get(string(constant.ContextKeyModelNameRPMChecked)); checked {
		return true
	}

	decision := setting.MatchModelNameRPM(modelName, policyGroup)
	if !decision.Matched {
		markModelNameRPMChecked(c)
		return true
	}

	buckets := []model_name_limiter.Bucket{{
		Key: model_name_limiter.ModelKey(decision.RuleModel), Limit: decision.GlobalRPM, Scope: "global",
	}}
	if decision.GroupRPM > 0 {
		buckets = append(buckets, model_name_limiter.Bucket{
			Key: model_name_limiter.GroupKey(decision.RuleModel, policyGroup), Limit: decision.GroupRPM, Scope: "group",
		})
	}
	if decision.UserRPM > 0 {
		userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		if userID <= 0 {
			common.SysError(fmt.Sprintf(
				"model_name_rpm: missing authenticated user id for user bucket request_id=%s rule_model=%s route=%s",
				c.GetString(common.RequestIdKey), decision.RuleModel, route,
			))
		} else {
			buckets = append(buckets, model_name_limiter.Bucket{
				Key: model_name_limiter.UserKey(decision.RuleModel, userID), Limit: decision.UserRPM, Scope: "user",
			})
		}
	}

	result := modelNameRPMAcquire(c.Request.Context(), buckets)
	if result.Allowed {
		markModelNameRPMChecked(c)
		return true
	}

	// A rejection is also terminal for this request. Mark before writing the
	// response so any downstream or deferred path cannot acquire a second time.
	markModelNameRPMChecked(c)
	requestID := c.GetString(common.RequestIdKey)
	reason := "rpm_limit_exceeded"
	logger.LogWarn(c.Request.Context(), fmt.Sprintf(
		"model_name_rpm rate limited: request_id=%s rule_model=%s policy_group=%s scope=%s limit=%d current=%d route=%s reason=%s",
		requestID, decision.RuleModel, policyGroup, result.Scope, result.Limit, result.Current, route, reason,
	))

	message := i18n.T(c, i18n.MsgModelNameRateLimited)
	if result.Scope == "user" {
		message = i18n.T(c, i18n.MsgModelNameUserRateLimited)
	}
	if responseMode == modelNameRPMTaskResponse {
		writeModelNameRPMTaskError(c, message)
	} else {
		writeModelNameRPMOpenAIError(c, message)
	}
	return false
}

func markModelNameRPMChecked(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyModelNameRPMChecked, true)
}

// Use 503 per project convention so downstream clients retry; clients use
// code == "model:rate_limited" to distinguish this gate from other 503s.
func writeModelNameRPMOpenAIError(c *gin.Context, message string) {
	recordRelayRejection(c, realtimemetrics.RejectionModelRPM)
	c.Header("Retry-After", modelNameRPMRetryAfter)
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "new_api_error",
			"code":    string(types.ErrorCodeModelNameRateLimited),
		},
	})
	c.Abort()
}

func writeModelNameRPMTaskError(c *gin.Context, message string) {
	recordRelayRejection(c, realtimemetrics.RejectionModelRPM)
	c.Header("Retry-After", modelNameRPMRetryAfter)
	taskErr := &dto.TaskError{
		Code:       string(types.ErrorCodeModelNameRateLimited),
		Message:    message,
		StatusCode: http.StatusServiceUnavailable,
		LocalError: true,
		Error:      errors.New(message),
	}
	c.JSON(http.StatusServiceUnavailable, taskErr)
	c.Abort()
}
