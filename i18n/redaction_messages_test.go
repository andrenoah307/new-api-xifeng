package i18n

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactionMessagesAreStaticAcrossLocales(t *testing.T) {
	require.NoError(t, Init())

	tests := []struct {
		name string
		key  string
		want map[string]string
	}{
		{
			name: "channel rate limited",
			key:  MsgChannelRateLimited,
			want: map[string]string{
				LangZhCN: "当前分组下请求过于频繁，请稍后重试",
				LangZhTW: "目前分組下請求過於頻繁，請稍後重試",
				LangEn:   "Requests in the current group are too frequent. Please try again later.",
			},
		},
		{
			name: "channel no available",
			key:  MsgChannelNoAvailable,
			want: map[string]string{
				LangZhCN: "当前分组下暂无可用渠道，请稍后重试",
				LangZhTW: "目前分組下暫無可用通道，請稍後重試",
				LangEn:   "No available channel for the current group. Please try again later.",
			},
		},
		{
			name: "deprecated group",
			key:  MsgGroupDeprecated,
			want: map[string]string{
				LangZhCN: "当前令牌所属分组已停用，请联系管理员",
				LangZhTW: "目前令牌所屬分組已停用，請聯絡管理員",
				LangEn:   "The group bound to this token has been retired. Please contact the administrator.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for language, want := range tt.want {
				got := Translate(language, tt.key)
				assert.Equal(t, want, got)
				assert.NotContains(t, got, "{{")
			}
		})
	}
}

func TestRedactionMessageKeysDoNotExposeTemplateData(t *testing.T) {
	require.NoError(t, Init())
	for _, key := range []string{MsgChannelRateLimited, MsgChannelNoAvailable, MsgGroupDeprecated} {
		for _, language := range []string{LangZhCN, LangZhTW, LangEn} {
			assert.False(t, strings.Contains(Translate(language, key), "{{"), "%s/%s must be static", language, key)
		}
	}
}

func TestRetiredTemplateMessageKeysAreAbsentFromAllLocales(t *testing.T) {
	require.NoError(t, Init())
	retiredKeys := []string{
		"channel.get_available_failed",
		"distributor.get_channel_failed",
		"distributor.no_available_channel",
	}
	for _, locale := range []string{"zh-CN", "zh-TW", "en"} {
		contents, err := localeFS.ReadFile("locales/" + locale + ".yaml")
		require.NoError(t, err)
		for _, key := range retiredKeys {
			assert.NotContains(t, string(contents), key+":")
		}
	}
}
