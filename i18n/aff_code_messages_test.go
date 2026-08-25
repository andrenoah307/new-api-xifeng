package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffCodeGenerationFailureMessageIsLocalized(t *testing.T) {
	require.NoError(t, Init())

	tests := map[string]string{
		LangEn:   "Failed to generate affiliate code. Please try again.",
		LangZhCN: "生成推广码失败，请重试",
		LangZhTW: "生成推廣碼失敗，請重試",
	}
	for language, want := range tests {
		assert.Equal(t, want, Translate(language, MsgUserAffCodeGenerateFailed))
	}
}
