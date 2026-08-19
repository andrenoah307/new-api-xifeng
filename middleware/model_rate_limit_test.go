package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/setting"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestA1RateLimitRejectionsAreLocalizedRedactedAndRetryable(t *testing.T) {
	require.NoError(t, i18n.Init())

	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	setting.ModelRequestRateLimitDurationMinutes = 7
	t.Cleanup(func() { setting.ModelRequestRateLimitDurationMinutes = previousDuration })
	const windowSeconds = int64(7 * 60)

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() {
		common.RDB = previousClient
		require.NoError(t, client.Close())
	})

	languages := []struct {
		name           string
		code           string
		successMessage string
		totalMessage   string
	}{
		{
			name:           "English",
			code:           i18n.LangEn,
			successMessage: "Too many successful requests. Please try again later.",
			totalMessage:   "Too many requests, including failed attempts. Please try again later.",
		},
		{
			name:           "Simplified Chinese",
			code:           i18n.LangZhCN,
			successMessage: "成功请求过于频繁，请稍后重试",
			totalMessage:   "总请求过于频繁（包括失败请求），请稍后重试",
		},
		{
			name:           "Traditional Chinese",
			code:           i18n.LangZhTW,
			successMessage: "成功請求過於頻繁，請稍後重試",
			totalMessage:   "總請求過於頻繁（包括失敗請求），請稍後重試",
		},
	}
	rejectionPoints := []struct {
		name           string
		useRedis       bool
		totalMax       int
		successMax     int
		isSuccessLimit bool
		messageKey     string
	}{
		{
			name:           "Redis successful request limit",
			useRedis:       true,
			totalMax:       0,
			successMax:     1,
			isSuccessLimit: true,
			messageKey:     i18n.MsgRateLimitReached,
		},
		{
			name:       "Redis total request limit",
			useRedis:   true,
			totalMax:   1,
			successMax: 100,
			messageKey: i18n.MsgRateLimitTotalReached,
		},
		{
			name:       "memory total request limit",
			useRedis:   false,
			totalMax:   1,
			successMax: 100,
			messageKey: i18n.MsgRateLimitTotalReached,
		},
		{
			name:           "memory successful request limit",
			useRedis:       false,
			totalMax:       0,
			successMax:     1,
			isSuccessLimit: true,
			messageKey:     i18n.MsgRateLimitReached,
		},
	}

	userID := 0
	for _, language := range languages {
		for _, rejectionPoint := range rejectionPoints {
			t.Run(language.name+"/"+rejectionPoint.name, func(t *testing.T) {
				userID--
				expectedMessage := language.totalMessage
				if rejectionPoint.isSuccessLimit {
					expectedMessage = language.successMessage
				}
				localizedMessage := i18n.Translate(language.code, rejectionPoint.messageKey)
				assert.Equal(t, expectedMessage, localizedMessage)
				assert.False(t, strings.ContainsFunc(localizedMessage, unicode.IsDigit))

				var handler gin.HandlerFunc
				if rejectionPoint.useRedis {
					handler = redisRateLimitHandler(windowSeconds, rejectionPoint.totalMax, rejectionPoint.successMax)
				} else {
					handler = memoryRateLimitHandler(windowSeconds, rejectionPoint.totalMax, rejectionPoint.successMax)
				}

				gin.SetMode(gin.TestMode)
				engine := gin.New()
				handled := 0
				engine.Use(func(c *gin.Context) {
					c.Set("id", userID)
					c.Set(string(constant.ContextKeyLanguage), language.code)
					c.Set(common.RequestIdKey, "rpm-a-one")
				})
				engine.GET("/v1/chat/completions", handler, func(c *gin.Context) {
					handled++
					c.Status(http.StatusOK)
				})

				first := httptest.NewRecorder()
				engine.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
				require.Equal(t, http.StatusOK, first.Code)

				rejected := httptest.NewRecorder()
				engine.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))
				assert.Equal(t, http.StatusServiceUnavailable, rejected.Code)
				assert.Equal(t, "420", rejected.Header().Get("Retry-After"))
				assert.Equal(t, 1, handled)

				var response struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, common.Unmarshal(rejected.Body.Bytes(), &response))
				assert.Equal(t, localizedMessage+" (request id: rpm-a-one)", response.Error.Message)
			})
		}
	}
}
