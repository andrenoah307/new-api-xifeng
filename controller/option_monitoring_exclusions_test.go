package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringExclusionOptionValidation(t *testing.T) {
	db := setupOptionControllerTest(t)
	t.Cleanup(func() {
		require.NoError(t, model.UpdateOption("group_monitoring_setting.excluded_channel_ids", "[]"))
		require.NoError(t, model.UpdateOption("group_monitoring_setting.excluded_user_ids", "[]"))
	})
	for _, key := range []string{"excluded_channel_ids", "excluded_user_ids"} {
		key = "group_monitoring_setting." + key
		for _, invalid := range []string{"[0]", "[-1]", "[1.5]", "[2147483648]", "[1,\"oops\"]", "{}"} {
			body := fmt.Sprintf(`{"key":%q,"value":%q}`, key, invalid)
			response := decodeBody(t, putSingleOption(t, body))
			assert.Equal(t, false, response["success"], invalid)
			_, exists := optionValueInDB(t, db, key)
			assert.False(t, exists, "invalid input must not persist")
		}
		for _, value := range []string{"[12,34]", "[]"} {
			body := fmt.Sprintf(`{"options":[{"key":%q,"value":%q}]}`, key, value)
			response := decodeBody(t, postOptionBatch(t, body))
			require.Equal(t, true, response["success"])
			stored, exists := optionValueInDB(t, db, key)
			require.True(t, exists)
			assert.Equal(t, value, stored)
			if key == "group_monitoring_setting.excluded_channel_ids" {
				assert.Equal(t, value != "[]", operation_setting.IsMonitoringExcluded(12, 0))
			} else {
				assert.Equal(t, value != "[]", operation_setting.IsMonitoringExcluded(0, 12))
			}
		}
	}
}

func TestMonitoringCacheScopeOptions(t *testing.T) {
	db := setupOptionControllerTest(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	for _, filter := range []string{"non_stream", "non_affinity"} {
		prefix := "group_monitoring_setting.cache_hit_exclude_" + filter
		for _, tc := range []struct {
			suffix         string
			invalid, valid []string
		}{
			{"_scope", []string{"unknown", "ALL", "[]"}, []string{"", "all", "selected"}},
			{"_groups", []string{"{}", `[1]`, `[""]`, `[" "]`}, []string{`["alpha","beta"]`, `[]`}},
		} {
			key := prefix + tc.suffix
			for _, value := range tc.invalid {
				response := decodeBody(t, putSingleOption(t, fmt.Sprintf(`{"key":%q,"value":%q}`, key, value)))
				assert.Equal(t, false, response["success"])
				_, exists := optionValueInDB(t, db, key)
				assert.False(t, exists)
			}
			for _, value := range tc.valid {
				response := decodeBody(t, postOptionBatch(t, fmt.Sprintf(`{"options":[{"key":%q,"value":%q}]}`, key, value)))
				require.Equal(t, true, response["success"])
				stored, exists := optionValueInDB(t, db, key)
				require.True(t, exists)
				assert.Equal(t, value, stored)
			}
		}
	}
}
