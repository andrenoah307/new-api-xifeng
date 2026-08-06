package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureRelayAdmissionTest(t *testing.T, maxRequests int, maxBodyBytes int64) {
	t.Helper()

	previousMaxRequests := common.RelayMaxConcurrentRequests
	previousMaxBodyBytes := common.RelayMaxActiveBodyBytes
	previousHighPercent := common.RelayMemoryBreakerHighPercent
	previousLowPercent := common.RelayMemoryBreakerLowPercent
	previousMaxTripSeconds := common.RelayMemoryBreakerMaxTripSeconds
	previousRetryAfter := common.RelayAdmissionRetryAfterSeconds
	previousRejectedConcurrent := relayAdmissionRejectedConcurrent.Load()
	previousRejectedBody := relayAdmissionRejectedBody.Load()
	previousRejectedMemory := relayAdmissionRejectedMemory.Load()
	previousExemptedMemory := relayAdmissionExemptedMemoryPressure.Load()

	common.RelayMaxConcurrentRequests = maxRequests
	common.RelayMaxActiveBodyBytes = maxBodyBytes
	common.RelayMemoryBreakerHighPercent = 0
	common.RelayMemoryBreakerLowPercent = 75
	common.RelayMemoryBreakerMaxTripSeconds = 0
	common.RelayAdmissionRetryAfterSeconds = 5
	relayAdmissionActiveRequests.Store(0)
	relayAdmissionActiveBodyBytes.Store(0)
	relayAdmissionRejectedConcurrent.Store(0)
	relayAdmissionRejectedBody.Store(0)
	relayAdmissionRejectedMemory.Store(0)
	relayAdmissionExemptedMemoryPressure.Store(0)

	t.Cleanup(func() {
		common.RelayMaxConcurrentRequests = previousMaxRequests
		common.RelayMaxActiveBodyBytes = previousMaxBodyBytes
		common.RelayMemoryBreakerHighPercent = previousHighPercent
		common.RelayMemoryBreakerLowPercent = previousLowPercent
		common.RelayMemoryBreakerMaxTripSeconds = previousMaxTripSeconds
		common.RelayAdmissionRetryAfterSeconds = previousRetryAfter
		relayAdmissionActiveRequests.Store(0)
		relayAdmissionActiveBodyBytes.Store(0)
		relayAdmissionRejectedConcurrent.Store(previousRejectedConcurrent)
		relayAdmissionRejectedBody.Store(previousRejectedBody)
		relayAdmissionRejectedMemory.Store(previousRejectedMemory)
		relayAdmissionExemptedMemoryPressure.Store(previousExemptedMemory)
	})
}

func newRelayAdmissionTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RelayAdmission())
	router.Any("/*path", handler)
	return router
}

func TestRelayAdmissionDisabledByDefault(t *testing.T) {
	for _, env := range []string{
		"RELAY_MAX_CONCURRENT_REQUESTS",
		"RELAY_MAX_ACTIVE_BODY_BYTES",
		"RELAY_MEMORY_BREAKER_HIGH_PERCENT",
		"RELAY_MEMORY_BREAKER_LOW_PERCENT",
		"RELAY_MEMORY_BREAKER_MAX_TRIP_SECONDS",
		"RELAY_ADMISSION_RETRY_AFTER_SECONDS",
	} {
		t.Setenv(env, "")
	}
	assert.Zero(t, common.GetEnvOrDefault("RELAY_MAX_CONCURRENT_REQUESTS", 0))
	assert.Zero(t, common.GetEnvOrDefaultInt64("RELAY_MAX_ACTIVE_BODY_BYTES", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_MEMORY_BREAKER_HIGH_PERCENT", 0))
	assert.Equal(t, 75, common.GetEnvOrDefault("RELAY_MEMORY_BREAKER_LOW_PERCENT", 75))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_MEMORY_BREAKER_MAX_TRIP_SECONDS", 0))
	assert.Equal(t, 5, common.GetEnvOrDefault("RELAY_ADMISSION_RETRY_AFTER_SECONDS", 5))

	configureRelayAdmissionTest(t, 0, 0)
	relayAdmissionActiveRequests.Store(7)
	relayAdmissionActiveBodyBytes.Store(11)

	var handled atomic.Int64
	router := newRelayAdmissionTestRouter(func(c *gin.Context) {
		handled.Add(1)
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("request body"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	assert.EqualValues(t, 1, handled.Load())
	assert.EqualValues(t, 7, relayAdmissionActiveRequests.Load())
	assert.EqualValues(t, 11, relayAdmissionActiveBodyBytes.Load())
}

func TestRelayAdmissionConcurrentRequestLimitAndRelease(t *testing.T) {
	configureRelayAdmissionTest(t, 2, 0)

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	router := newRelayAdmissionTestRouter(func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	type requestResult struct {
		status int
	}
	results := make(chan requestResult, 2)
	for range 2 {
		go func() {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			router.ServeHTTP(recorder, request)
			results <- requestResult{status: recorder.Code}
		}()
	}
	<-entered
	<-entered

	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
	assert.Equal(t, "5", rejected.Header().Get("Retry-After"))
	assert.EqualValues(t, 2, relayAdmissionActiveRequests.Load())

	close(release)
	for range 2 {
		assert.Equal(t, http.StatusNoContent, (<-results).status)
	}
	require.Zero(t, relayAdmissionActiveRequests.Load())

	recovered := httptest.NewRecorder()
	router.ServeHTTP(recovered, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	assert.Equal(t, http.StatusNoContent, recovered.Code)
	assert.Zero(t, relayAdmissionActiveRequests.Load())
}

func TestRelayAdmissionActiveBodyBudget(t *testing.T) {
	configureRelayAdmissionTest(t, 0, 10)

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	router := newRelayAdmissionTestRouter(func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	results := make(chan int, 2)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("123456"))
		router.ServeHTTP(recorder, request)
		results <- recorder.Code
	}()
	<-entered
	require.EqualValues(t, 6, relayAdmissionActiveBodyBytes.Load())

	rejected := httptest.NewRecorder()
	rejectedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("12345"))
	router.ServeHTTP(rejected, rejectedRequest)
	require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
	assert.EqualValues(t, 6, relayAdmissionActiveBodyBytes.Load())

	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		request.Body = io.NopCloser(strings.NewReader("unknown length body"))
		request.ContentLength = -1
		router.ServeHTTP(recorder, request)
		results <- recorder.Code
	}()
	<-entered
	assert.EqualValues(t, 6, relayAdmissionActiveBodyBytes.Load(), "unknown-length bodies must not reserve the byte budget")

	close(release)
	for range 2 {
		assert.Equal(t, http.StatusNoContent, <-results)
	}
	assert.Zero(t, relayAdmissionActiveBodyBytes.Load())

	recovered := httptest.NewRecorder()
	recoveredRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("1234567890"))
	router.ServeHTTP(recovered, recoveredRequest)
	assert.Equal(t, http.StatusNoContent, recovered.Code)
	assert.Zero(t, relayAdmissionActiveBodyBytes.Load())
}

func TestRelayAdmissionErrorResponseFormats(t *testing.T) {
	configureRelayAdmissionTest(t, 1, 0)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	router := newRelayAdmissionTestRouter(func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	done := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/hold", nil))
		close(done)
	}()
	<-entered

	tests := []struct {
		name        string
		path        string
		wantCodeKey bool
	}{
		{name: "Claude messages", path: "/v1/messages", wantCodeKey: false},
		{name: "OpenAI chat completions", path: "/v1/chat/completions", wantCodeKey: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path, nil))
			require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			assert.Equal(t, "5", recorder.Header().Get("Retry-After"))

			var response struct {
				Error map[string]any `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.NotNil(t, response.Error)
			assert.NotEmpty(t, response.Error["message"])
			_, hasCode := response.Error["code"]
			assert.Equal(t, test.wantCodeKey, hasCode)
			if test.wantCodeKey {
				assert.Equal(t, "too_many_concurrent_requests", response.Error["code"])
			}
		})
	}

	close(release)
	<-done
}

func TestRelayAdmissionSingleRequestAboveBodyBudgetIsIntentionalHardUpperBound(t *testing.T) {
	configureRelayAdmissionTest(t, 1, 10)
	var handled atomic.Bool
	router := newRelayAdmissionTestRouter(func(c *gin.Context) {
		handled.Store(true)
		c.Status(http.StatusNoContent)
	})

	for range 8 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("12345678901"))
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Equal(t, "5", recorder.Header().Get("Retry-After"))
		assert.False(t, handled.Load())
		assert.Zero(t, relayAdmissionActiveRequests.Load())
		assert.Zero(t, relayAdmissionActiveBodyBytes.Load())
	}
}

func TestRelayAdmissionMemoryPressureRejectsWithoutLeakingReservations(t *testing.T) {
	configureRelayAdmissionTest(t, 1, 10)
	common.RelayMemoryBreakerHighPercent = 80

	var samplerStarted atomic.Bool
	var handled atomic.Bool
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(newRelayAdmissionHandler(
		func() { samplerStarted.Store(true) },
		func() bool { return true },
	))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		handled.Store(true)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("12345"))
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.True(t, samplerStarted.Load())
	assert.False(t, handled.Load())
	assert.Zero(t, relayAdmissionActiveRequests.Load())
	assert.Zero(t, relayAdmissionActiveBodyBytes.Load())

	var response struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "memory_pressure", response.Error["code"])
}

func TestRelayAdmissionMemoryPressureExemption(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		path               string
		upgrade            string
		memoryPressure     bool
		wantStatus         int
		wantHandled        bool
		wantExemptedMemory uint64
		wantRejectedMemory uint64
	}{
		{
			name:               "GET task result is exempt while tripped",
			method:             http.MethodGet,
			path:               "/v1/videos/task-1/content",
			memoryPressure:     true,
			wantStatus:         http.StatusNoContent,
			wantHandled:        true,
			wantExemptedMemory: 1,
		},
		{
			name:               "lowercase websocket upgrade remains gated",
			method:             http.MethodGet,
			path:               "/v1/realtime",
			upgrade:            "websocket",
			memoryPressure:     true,
			wantStatus:         http.StatusServiceUnavailable,
			wantRejectedMemory: 1,
		},
		{
			name:               "mixed case websocket upgrade remains gated",
			method:             http.MethodGet,
			path:               "/v1/realtime",
			upgrade:            "WebSocket",
			memoryPressure:     true,
			wantStatus:         http.StatusServiceUnavailable,
			wantRejectedMemory: 1,
		},
		{
			name:               "POST remains gated",
			method:             http.MethodPost,
			path:               "/v1/chat/completions",
			memoryPressure:     true,
			wantStatus:         http.StatusServiceUnavailable,
			wantRejectedMemory: 1,
		},
		{
			name:           "GET without pressure follows normal path",
			method:         http.MethodGet,
			path:           "/v1/videos/task-1",
			memoryPressure: false,
			wantStatus:     http.StatusNoContent,
			wantHandled:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureRelayAdmissionTest(t, 1, 10)
			common.RelayMemoryBreakerHighPercent = 80

			var handled atomic.Bool
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(newRelayAdmissionHandler(func() {}, func() bool { return test.memoryPressure }))
			router.Any("/*path", func(c *gin.Context) {
				handled.Store(true)
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(test.method, test.path, nil)
			if test.upgrade != "" {
				request.Header.Set("Upgrade", test.upgrade)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			require.Equal(t, test.wantStatus, recorder.Code)
			assert.Equal(t, test.wantHandled, handled.Load())
			stats := GetRelayAdmissionStats()
			assert.Equal(t, test.wantExemptedMemory, stats.ExemptedMemoryPressure)
			assert.Equal(t, test.wantRejectedMemory, stats.RejectedMemoryPressure)
			assert.Zero(t, stats.RejectedTooManyConcurrentRequests)
			assert.Zero(t, stats.RejectedRequestBodyBudgetExhausted)
			assert.Zero(t, stats.ActiveRequests)
			assert.Zero(t, stats.ActiveBodyBytes)
		})
	}
}

func TestRelayAdmissionMemoryPressureExemptRequestReleasesReservations(t *testing.T) {
	configureRelayAdmissionTest(t, 1, 10)
	common.RelayMemoryBreakerHighPercent = 80

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(newRelayAdmissionHandler(func() {}, func() bool { return true }))
	router.GET("/v1/videos/:task_id", func(c *gin.Context) {
		entered <- struct{}{}
		<-release
		c.Status(http.StatusNoContent)
	})

	result := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/videos/task-1", strings.NewReader("12345"))
		router.ServeHTTP(recorder, request)
		result <- recorder.Code
	}()
	<-entered

	stats := GetRelayAdmissionStats()
	assert.EqualValues(t, 1, stats.ActiveRequests)
	assert.EqualValues(t, 5, stats.ActiveBodyBytes)
	assert.EqualValues(t, 1, stats.ExemptedMemoryPressure)
	assert.Zero(t, stats.RejectedTooManyConcurrentRequests)
	assert.Zero(t, stats.RejectedRequestBodyBudgetExhausted)
	assert.Zero(t, stats.RejectedMemoryPressure)

	close(release)
	require.Equal(t, http.StatusNoContent, <-result)
	assert.Zero(t, relayAdmissionActiveRequests.Load())
	assert.Zero(t, relayAdmissionActiveBodyBytes.Load())
}

func TestRejectRelayAdmissionCountsEachReasonWithoutChangingResponseContract(t *testing.T) {
	previousRejectedConcurrent := relayAdmissionRejectedConcurrent.Load()
	previousRejectedBody := relayAdmissionRejectedBody.Load()
	previousRejectedMemory := relayAdmissionRejectedMemory.Load()
	previousExemptedMemory := relayAdmissionExemptedMemoryPressure.Load()
	relayAdmissionRejectedConcurrent.Store(0)
	relayAdmissionRejectedBody.Store(0)
	relayAdmissionRejectedMemory.Store(0)
	relayAdmissionExemptedMemoryPressure.Store(0)
	t.Cleanup(func() {
		relayAdmissionRejectedConcurrent.Store(previousRejectedConcurrent)
		relayAdmissionRejectedBody.Store(previousRejectedBody)
		relayAdmissionRejectedMemory.Store(previousRejectedMemory)
		relayAdmissionExemptedMemoryPressure.Store(previousExemptedMemory)
	})

	tests := []struct {
		name      string
		errorCode types.ErrorCode
		want      RelayAdmissionStats
	}{
		{
			name:      "concurrent request limit",
			errorCode: "too_many_concurrent_requests",
			want:      RelayAdmissionStats{RejectedTooManyConcurrentRequests: 1},
		},
		{
			name:      "body budget",
			errorCode: "request_body_budget_exhausted",
			want:      RelayAdmissionStats{RejectedRequestBodyBudgetExhausted: 1},
		},
		{
			name:      "memory pressure",
			errorCode: "memory_pressure",
			want:      RelayAdmissionStats{RejectedMemoryPressure: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relayAdmissionRejectedConcurrent.Store(0)
			relayAdmissionRejectedBody.Store(0)
			relayAdmissionRejectedMemory.Store(0)
			relayAdmissionExemptedMemoryPressure.Store(0)
			relayAdmissionActiveRequests.Store(0)
			relayAdmissionActiveBodyBytes.Store(0)

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			rejectRelayAdmission(context, 5, test.errorCode, "rejected")

			require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			assert.Equal(t, "5", recorder.Header().Get("Retry-After"))
			var response struct {
				Error map[string]any `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, string(test.errorCode), response.Error["code"])

			stats := GetRelayAdmissionStats()
			assert.Equal(t, test.want.RejectedTooManyConcurrentRequests, stats.RejectedTooManyConcurrentRequests)
			assert.Equal(t, test.want.RejectedRequestBodyBudgetExhausted, stats.RejectedRequestBodyBudgetExhausted)
			assert.Equal(t, test.want.RejectedMemoryPressure, stats.RejectedMemoryPressure)
			assert.Zero(t, stats.ExemptedMemoryPressure)
			assert.Zero(t, stats.ActiveRequests)
			assert.Zero(t, stats.ActiveBodyBytes)
		})
	}
}
