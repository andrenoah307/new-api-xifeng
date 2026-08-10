package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminResetUserSubscriptionsWritesOneSubjectAudit(t *testing.T) {
	db := setupUserAccountAuditDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))

	admin := &model.User{
		Username: "subscription-audit-admin",
		Password: "password",
		AffCode:  "subscription-audit-admin-code",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	}
	target := &model.User{
		Username: "subscription-audit-target",
		Password: "password",
		AffCode:  "subscription-audit-target-code",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(target).Error)

	plan := &model.SubscriptionPlan{
		Title:            "Audit plan",
		PriceAmount:      10,
		DurationUnit:     model.SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, db.Create(plan).Error)
	now := model.GetDBTimestamp()
	require.NoError(t, db.Create(&model.UserSubscription{
		UserId:      target.Id,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  400,
		StartTime:   now - 3600,
		EndTime:     now + 30*86400,
		Status:      "active",
	}).Error)

	body, err := common.Marshal(AdminResetSubscriptionRequest{
		PlanId:           plan.Id,
		AdvanceResetTime: common.GetPointer(false),
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/admin/users/"+strconv.Itoa(target.Id)+"/reset", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
	ctx.Request.RemoteAddr = "198.51.100.88:4321"
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	ctx.Set("id", admin.Id)
	ctx.Set("username", admin.Username)
	ctx.Set("role", admin.Role)

	AdminResetUserSubscriptionsByPlan(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var logs []*model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, target.Id, logs[0].UserId)
	assert.Equal(t, target.Username, logs[0].Username)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	op, ok := other["op"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "subscription.user_plan_reset", op["action"])
	params, ok := op["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(target.Id), params["target_user_id"])
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(admin.Id), adminInfo["admin_id"])
}
