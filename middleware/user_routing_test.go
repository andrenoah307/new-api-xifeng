package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserRoutingProtocolsPlaygroundAffinityAndBilling(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldDB, oldMemory, oldRedis := model.DB, common.MemoryCacheEnabled, common.RedisEnabled
	oldUsable := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	affinity := operation_setting.GetChannelAffinitySetting()
	oldAffinity := *affinity
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "routing.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB, common.MemoryCacheEnabled, common.RedisEnabled = db, true, false
	t.Cleanup(func() {
		require.NoError(t, model.SaveUserRoutingRules(79, nil))
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = oldDB, oldMemory, oldRedis
		*affinity = oldAffinity
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.UserRoutingPolicy{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","source":"Source"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"source":2,"destination":50}`))
	require.NoError(t, model.SaveUserRoutingRules(79, []model.UserRoutingRule{{SourceGroup: "source", Model: "model-a", TargetGroup: "destination", Enabled: true}}))
	for index, group := range []string{"source", "destination"} {
		id := 9400 + index
		require.NoError(t, db.Create(&model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Name: group, Key: "test-key", Group: group, Models: "model-a", Status: common.ChannelStatusEnabled}).Error)
		require.NoError(t, db.Create(&model.Ability{ChannelId: id, Group: group, Model: "model-a", Enabled: true}).Error)
	}
	model.InitChannelCache()
	affinity.Enabled = true
	affinity.Rules = []operation_setting.ChannelAffinityRule{{Name: "user-routing-protocol-test", ModelRegex: []string{"^model-a$"}, IncludeRuleName: true, IncludeModelName: true, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Routing-Test"}}}}
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses", "/pg/chat/completions"} {
		for attempt := 0; attempt < 2; attempt++ {
			t.Run(fmt.Sprintf("%s/%d", path, attempt), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"model-a","group":"source"}`))
				c.Request.Header.Set("Content-Type", gin.MIMEJSON)
				c.Request.Header.Set("X-Routing-Test", path)
				c.Set("id", 79)
				c.Set("user_group", "default")
				c.Set("group", "source")
				if strings.HasPrefix(path, "/pg/") {
					c.Set("group", "default")
				}
				common.SetContextKey(c, constant.ContextKeyModelNameRPMChecked, true)
				Distribute()(c)
				require.False(t, c.IsAborted(), rec.Body.String())
				assert.Equal(t, 9401, c.GetInt("channel_id"))
				assert.Equal(t, "source", c.GetString("group"))
				info := &relaycommon.RelayInfo{UserGroup: "default", UsingGroup: c.GetString("group")}
				price := helper.HandleGroupRatio(c, info)
				assert.Equal(t, 2.0, price.GroupRatio, "destination's 50x ratio must never reach accounting")
				assert.Equal(t, "source", info.UsingGroup)
			})
		}
	}
}
