package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitCapacityThrottleTranslations(t *testing.T) {
	require.NoError(t, Init())

	testCases := []struct {
		language string
		message  string
	}{
		{language: LangEn, message: "Too many requests. Please try again later."},
		{language: LangZhCN, message: "请求过于频繁，请稍后重试"},
		{language: LangZhTW, message: "請求過於頻繁，請稍後重試"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.language, func(t *testing.T) {
			assert.Equal(t, testCase.message, Translate(testCase.language, MsgRateLimitCapacityThrottled))
		})
	}
}
