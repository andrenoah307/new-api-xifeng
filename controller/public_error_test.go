package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicErrorMessageOptionPersistsValidatedConfiguration(t *testing.T) {
	db := setupOptionControllerTest(t)
	previous, err := common.Marshal(common.GetUpstreamErrorMessages())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, common.UpdateUpstreamErrorMessages(string(previous))) })
	w := putSingleOption(t, `{"key":"UpstreamErrorMessages","value":"{\"client\":\"Check input\",\"server\":\"Try later\"}"}`)
	require.Equal(t, true, decodeBody(t, w)["success"])
	stored, exists := optionValueInDB(t, db, common.UpstreamErrorMessagesOption)
	require.True(t, exists)
	assert.Equal(t, "Check input", common.PublicErrorMessage(429, ""))
	assert.Equal(t, "Try later", common.PublicErrorMessage(503, ""))
	w = putSingleOption(t, `{"key":"UpstreamErrorMessages","value":"{\"server\":\"${upstream}\"}"}`)
	assert.Equal(t, false, decodeBody(t, w)["success"])
	after, exists := optionValueInDB(t, db, common.UpstreamErrorMessagesOption)
	require.True(t, exists)
	assert.Equal(t, stored, after)
	assert.Equal(t, "Try later", common.PublicErrorMessage(503, ""))
}

func TestTaskPublicErrorRetainsInternalDetails(t *testing.T) {
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Set(common.RequestIdKey, "local-task-id")
	original := &dto.TaskError{Code: "provider_private", Message: "group private-group unavailable", Data: map[string]string{"key": "private-key"}, StatusCode: http.StatusBadRequest}
	respondTaskError(c, original)
	assert.Equal(t, http.StatusBadRequest, r.Code)
	assert.Contains(t, r.Body.String(), "请求参数有误，请检查请求内容")
	assert.Contains(t, r.Body.String(), "local-task-id")
	assert.NotContains(t, r.Body.String(), "private")
	assert.Equal(t, "group private-group unavailable", original.Message)
	assert.Equal(t, "provider_private", original.Code)
	assert.Equal(t, map[string]string{"key": "private-key"}, original.Data)
}
