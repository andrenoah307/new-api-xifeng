package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOptionControllerTest(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}, &model.User{}))
	model.DB = db
	model.LOG_DB = db

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled = oldRedisEnabled
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	saveMonitoringSettings(t)
	return db
}

func postOptionBatch(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/batch", strings.NewReader(body))
	UpdateOptions(c)
	return w
}

func putSingleOption(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(body))
	UpdateOption(c)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(w.Body.String(), &payload))
	return payload
}

func optionValueInDB(t *testing.T, db *gorm.DB, key string) (string, bool) {
	t.Helper()
	var row model.Option
	err := db.Where("`key` = ?", key).First(&row).Error
	if err != nil {
		return "", false
	}
	return row.Value, true
}

// 一次批量请求必须把所有配置项都写进数据库和内存，且顺序与请求一致。
// 这是本次改动的核心诉求：管理员保存分组监控设置不再产生 17 次串行 HTTP 往返。
func TestUpdateOptions_AppliesEveryKey(t *testing.T) {
	db := setupOptionControllerTest(t)

	w := postOptionBatch(t, `{"options":[
		{"key":"group_monitoring_setting.perf_card_enabled","value":true},
		{"key":"group_monitoring_setting.perf_card_top_n","value":8},
		{"key":"group_monitoring_setting.perf_card_groups","value":"[\"default\"]"}
	]}`)

	require.Equal(t, http.StatusOK, w.Code)
	payload := decodeBody(t, w)
	require.Equal(t, true, payload["success"], w.Body.String())
	data := payload["data"].(map[string]any)
	assert.Equal(t, []any{
		"group_monitoring_setting.perf_card_enabled",
		"group_monitoring_setting.perf_card_top_n",
		"group_monitoring_setting.perf_card_groups",
	}, data["applied"], "applied 必须按请求顺序回报，供前端定位失败位置")

	for key, want := range map[string]string{
		"group_monitoring_setting.perf_card_enabled": "true",
		"group_monitoring_setting.perf_card_top_n":   "8",
		"group_monitoring_setting.perf_card_groups":  `["default"]`,
	} {
		value, ok := optionValueInDB(t, db, key)
		require.True(t, ok, "%s 必须落库", key)
		assert.Equal(t, want, value, "%s 落库值", key)
	}
}

// 批量写入必须复用单键端点的全部校验逻辑，不能因为走了新入口就绕过。
// 这里用「未配置 GitHub Client Id 时不得开启 GitHub OAuth」这条既有守卫做判别器。
func TestUpdateOptions_StopsAtFirstValidationFailure(t *testing.T) {
	db := setupOptionControllerTest(t)

	oldClientId := common.GitHubClientId
	common.GitHubClientId = ""
	t.Cleanup(func() { common.GitHubClientId = oldClientId })

	w := postOptionBatch(t, `{"options":[
		{"key":"group_monitoring_setting.perf_card_top_n","value":9},
		{"key":"GitHubOAuthEnabled","value":true},
		{"key":"group_monitoring_setting.perf_card_enabled","value":false}
	]}`)

	require.Equal(t, http.StatusOK, w.Code, "逐项业务校验失败沿用既有 200 语义，不是请求格式错误")
	payload := decodeBody(t, w)
	require.Equal(t, false, payload["success"])
	assert.Equal(t, "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！", payload["message"])

	data := payload["data"].(map[string]any)
	assert.Equal(t, []any{"group_monitoring_setting.perf_card_top_n"}, data["applied"], "已成功的键必须回报，前端才知道半应用的边界")
	failed := data["failed"].(map[string]any)
	assert.Equal(t, "GitHubOAuthEnabled", failed["key"])

	value, ok := optionValueInDB(t, db, "group_monitoring_setting.perf_card_top_n")
	require.True(t, ok, "失败前已应用的键必须保持写入（与单键端点逐个保存的语义一致）")
	assert.Equal(t, "9", value)
	_, ok = optionValueInDB(t, db, "group_monitoring_setting.perf_card_enabled")
	assert.False(t, ok, "失败点之后的键不得继续写入")
}

// 合规确认字段只能走专用端点。批量入口若漏掉这条守卫，管理员就能绕过支付合规确认。
func TestUpdateOptions_RejectsPaymentComplianceKeys(t *testing.T) {
	db := setupOptionControllerTest(t)

	w := postOptionBatch(t, `{"options":[{"key":"payment_setting.compliance_confirmed","value":"true"}]}`)

	require.Equal(t, http.StatusOK, w.Code)
	payload := decodeBody(t, w)
	require.Equal(t, false, payload["success"])
	assert.Equal(t, "合规确认字段不允许通过通用设置接口修改", payload["message"])
	_, ok := optionValueInDB(t, db, "payment_setting.compliance_confirmed")
	assert.False(t, ok)
}

// 请求格式错误（空列表 / 空键 / 重复键 / 超量）返回 400：这是管理员参数错误，
// 不能伪装成业务失败，也不能落任何一项。
func TestUpdateOptions_RejectsMalformedRequest(t *testing.T) {
	oversized := make([]string, 0, maxBatchOptionUpdates+1)
	for i := 0; i <= maxBatchOptionUpdates; i++ {
		oversized = append(oversized, fmt.Sprintf(`{"key":"K%d","value":"v"}`, i))
	}

	cases := []struct {
		name string
		body string
	}{
		{"非法 JSON", `{"options":`},
		{"空列表", `{"options":[]}`},
		{"缺少 options", `{}`},
		{"空键名", `{"options":[{"key":"","value":"v"}]}`},
		{"重复键名", `{"options":[{"key":"SystemName","value":"a"},{"key":"SystemName","value":"b"}]}`},
		{"超过上限", `{"options":[` + strings.Join(oversized, ",") + `]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupOptionControllerTest(t)
			w := postOptionBatch(t, tc.body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, false, decodeBody(t, w)["success"])

			var count int64
			require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
			assert.Zero(t, count, "请求格式错误时不得写入任何配置项")
		})
	}
}

// 审计：批量的每个键都要独立成行（沿用单键端点每次保存一条的粒度），
// 且同一批共享 batch_id，便于事后把一次保存动作还原成整体。
func TestUpdateOptions_RecordsOneAuditPerKey(t *testing.T) {
	db := setupOptionControllerTest(t)

	w := postOptionBatch(t, `{"options":[
		{"key":"group_monitoring_setting.perf_card_top_n","value":7},
		{"key":"group_monitoring_setting.perf_card_enabled","value":true}
	]}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, true, decodeBody(t, w)["success"], w.Body.String())

	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	require.Len(t, logs, 2, "每个成功写入的键各一条审计")

	batchIds := map[string]bool{}
	for _, entry := range logs {
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(entry.Other, &other))
		op := other["op"].(map[string]any)
		params := op["params"].(map[string]any)
		assert.NotEmpty(t, params["key"])
		assert.NotContains(t, entry.Content, "value", "审计只记录键名，不得记录可能含密钥的配置值")
		batchId, ok := params["batch_id"].(string)
		require.True(t, ok, "批量审计必须带 batch_id")
		batchIds[batchId] = true
	}
	assert.Len(t, batchIds, 1, "同一批必须共享同一个 batch_id")
}

// 回归：把校验逻辑抽成 applyOptionUpdate 之后，单键端点的响应必须逐字节不变。
func TestUpdateOption_SingleKeyContractUnchanged(t *testing.T) {
	setupOptionControllerTest(t)

	oldClientId := common.GitHubClientId
	common.GitHubClientId = ""
	t.Cleanup(func() { common.GitHubClientId = oldClientId })

	success := putSingleOption(t, `{"key":"group_monitoring_setting.perf_card_top_n","value":5}`)
	assert.Equal(t, http.StatusOK, success.Code)
	assert.JSONEq(t, `{"success":true,"message":""}`, success.Body.String())

	businessFailure := putSingleOption(t, `{"key":"GitHubOAuthEnabled","value":true}`)
	assert.Equal(t, http.StatusOK, businessFailure.Code)
	assert.JSONEq(t, `{"success":false,"message":"无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！"}`, businessFailure.Body.String())

	complianceFailure := putSingleOption(t, `{"key":"payment_setting.compliance_confirmed","value":"true"}`)
	assert.Equal(t, http.StatusOK, complianceFailure.Code)
	assert.JSONEq(t, `{"success":false,"message":"合规确认字段不允许通过通用设置接口修改"}`, complianceFailure.Body.String())

	malformed := putSingleOption(t, `{"key":`)
	assert.Equal(t, http.StatusBadRequest, malformed.Code)
	assert.JSONEq(t, `{"success":false,"message":"无效的参数"}`, malformed.Body.String())
}
