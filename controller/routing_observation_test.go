package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingObservationRetryDecision(t *testing.T) {
	previous := operation_setting.AutomaticRetryStatusCodesToString()
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500-503"))
	t.Cleanup(func() { require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(previous)) })
	for _, test := range []struct {
		name              string
		status, budget    int
		force, skip, want bool
		reason            string
	}{
		{"parameter error", 400, 2, false, false, false, "status_policy"},
		{"server error", 500, 2, false, false, true, "status_policy"},
		{"exhausted", 500, 0, false, false, false, "retry_budget_exhausted"},
		{"explicit skip", 500, 2, false, true, false, "skip_retry_error"},
		{"forced retry retains precedence", 400, 0, true, true, true, "forced_by_error_filter"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("channel_id", 17)
			service.BeginRoutingObservation(c, "claude-test")
			err := types.NewErrorWithStatusCode(errors.New("private upstream message"), types.ErrorCodeBadResponseStatusCode, test.status)
			err.SetSkipRetry(test.skip)
			err.SetForceRetry(test.force)
			assert.Equal(t, test.want, shouldRetry(c, err, test.budget))
			snapshot := service.RoutingObservationSnapshot(c)
			require.Len(t, snapshot.Events, 1)
			assert.Equal(t, test.reason, snapshot.Events[0].Reason)
			assert.Equal(t, test.status, snapshot.Events[0].Status)
			assert.Equal(t, 17, snapshot.Events[0].Channel)
			kind := "retry_denied"
			if test.want {
				kind = "retry_allowed"
			}
			assert.Equal(t, kind, snapshot.Events[0].Kind)
		})
	}
}

func TestRoutingObservationErrorFilterKeepsANDMatching(t *testing.T) {
	rules := []dto.ErrorFilterRule{{Action: "retry", StatusCodes: []int{400}, MessageContains: []string{"capacity"}}}
	err := types.NewErrorWithStatusCode(errors.New("unsupported parameter"), types.ErrorCodeBadResponseStatusCode, 400)
	index, action := applyChannelErrorFilter(err, rules, 400, "", "unsupported parameter")
	assert.Equal(t, 0, index)
	assert.Equal(t, "no_match", action)
	assert.False(t, err.IsForceRetry())
	index, action = applyChannelErrorFilter(err, rules, 400, "", "capacity exceeded")
	assert.Equal(t, 1, index)
	assert.Equal(t, "retry", action)
	assert.True(t, err.IsForceRetry())
}
