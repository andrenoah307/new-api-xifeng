package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUICustomizationOptionsPersistNormalizedValues(t *testing.T) {
	db := setupOptionControllerTest(t)
	for _, tc := range []struct{ key, input, want string }{
		{"TicketQuickReplies", " First \r\n\nSecond\nFirst ", "First\nSecond"},
		{"CustomNavigationLinks", "null", "[]"},
		{"HeaderNavStyle", "packy", "packy"},
		{"HeaderNavStyle", "default", "default"},
	} {
		response := decodeBody(t, putSingleOption(t, fmt.Sprintf(`{"key":%q,"value":%q}`, tc.key, tc.input)))
		require.Equal(t, true, response["success"])
		stored, exists := optionValueInDB(t, db, tc.key)
		require.True(t, exists)
		assert.Equal(t, tc.want, stored)
		common.OptionMapRWMutex.RLock()
		cached := common.OptionMap[tc.key]
		common.OptionMapRWMutex.RUnlock()
		assert.Equal(t, tc.want, cached)
	}
}

func TestUICustomizationRejectsUnsafeValuesBeforePersistence(t *testing.T) {
	db := setupOptionControllerTest(t)
	for _, tc := range []struct{ key, value string }{
		{"TicketQuickReplies", strings.Repeat("x", 5001)},
		{"HeaderNavStyle", "custom-css"},
		{"CustomNavigationLinks", `[{"id":"bad","title":"Bad","url":"javascript:alert(1)","mode":"external","header":true}]`},
	} {
		response := decodeBody(t, putSingleOption(t, fmt.Sprintf(`{"key":%q,"value":%q}`, tc.key, tc.value)))
		assert.Equal(t, false, response["success"])
		_, exists := optionValueInDB(t, db, tc.key)
		assert.False(t, exists)
	}
}
