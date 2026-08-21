package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	backendi18n "github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredLogQueryTimeout(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "default", value: "", want: 30 * time.Second},
		{name: "custom", value: "45", want: 45 * time.Second},
		{name: "zero disables deadline", value: "0", want: 0},
		{name: "negative disables deadline", value: "-1", want: -time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LOG_QUERY_TIMEOUT", test.value)
			assert.Equal(t, test.want, configuredLogQueryTimeout())
		})
	}
}

func TestGetStatusIncludesConfiguredLogQueryTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "positive timeout", value: "60", want: 60},
		{name: "zero disables deadline", value: "0", want: 0},
		{name: "negative disables deadline", value: "-1", want: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LOG_QUERY_TIMEOUT", test.value)
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

			GetStatus(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload struct {
				Data struct {
					LogQueryTimeout int `json:"log_query_timeout"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Equal(t, test.want, payload.Data.LogQueryTimeout)
		})
	}
}

func TestNewLogQueryContextDeadlineBehavior(t *testing.T) {
	t.Run("positive timeout adds deadline", func(t *testing.T) {
		t.Setenv("LOG_QUERY_TIMEOUT", "")
		ctx, cancel := newLogQueryContext(context.Background())
		defer cancel()

		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
	})

	for _, value := range []string{"0", "-1"} {
		t.Run("disabled timeout "+value, func(t *testing.T) {
			t.Setenv("LOG_QUERY_TIMEOUT", value)
			parent, cancelParent := context.WithCancel(context.Background())
			ctx, cancel := newLogQueryContext(parent)
			defer cancel()

			_, hasDeadline := ctx.Deadline()
			assert.False(t, hasDeadline)

			cancelParent()
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	}
}

func TestHandleLogQueryError(t *testing.T) {
	require.NoError(t, backendi18n.Init())

	tests := []struct {
		name        string
		language    string
		err         error
		wantStatus  int
		wantMessage string
		wantBody    bool
	}{
		{
			name:        "deadline exceeded in English",
			language:    backendi18n.LangEn,
			err:         fmt.Errorf("database query: %w", context.DeadlineExceeded),
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "Query timed out. Add more filters or narrow the time range, then try again.",
			wantBody:    true,
		},
		{
			name:        "deadline exceeded in Chinese",
			language:    backendi18n.LangZhCN,
			err:         context.DeadlineExceeded,
			wantStatus:  http.StatusServiceUnavailable,
			wantMessage: "查询超时，请增加筛选条件或缩小时间范围后重试",
			wantBody:    true,
		},
		{
			name:     "client canceled",
			language: backendi18n.LangEn,
			err:      fmt.Errorf("database query: %w", context.Canceled),
			wantBody: false,
		},
		{
			name:        "other database error keeps legacy response",
			language:    backendi18n.LangEn,
			err:         errors.New("database unavailable"),
			wantStatus:  http.StatusOK,
			wantMessage: "database unavailable",
			wantBody:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log", nil)
			ctx.Request.Header.Set("Accept-Language", test.language)

			handleLogQueryError(ctx, test.err)

			if !test.wantBody {
				assert.False(t, ctx.Writer.Written())
				assert.Empty(t, recorder.Body.String())
				return
			}

			require.Equal(t, test.wantStatus, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.wantMessage, response.Message)
		})
	}
}
