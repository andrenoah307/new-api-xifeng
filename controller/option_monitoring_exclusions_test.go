package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
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
