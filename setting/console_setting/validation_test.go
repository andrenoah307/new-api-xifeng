package console_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAnnouncementsLegacyFormatStillPasses(t *testing.T) {
	// 存量公告（无多语言字段）必须继续通过校验——向后兼容契约
	legacy := `[{"id":1,"content":"旧公告内容","publishDate":"2026-07-01T00:00:00Z","type":"default","extra":"备注"}]`
	require.NoError(t, ValidateConsoleSettings(legacy, "Announcements"))
}

func TestValidateAnnouncementsLongContentAllowed(t *testing.T) {
	// 500 字符上限已移除（原实现按字节计，500 字节 ≈ 166 个汉字）
	long := strings.Repeat("公告很长", 500)
	require.Greater(t, len(long), 500)
	jsonStr := `[{"content":"` + long + `","publishDate":"2026-07-01T00:00:00Z"}]`
	require.NoError(t, ValidateConsoleSettings(jsonStr, "Announcements"))
}

func TestValidateAnnouncementsContentI18n(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name: "valid contentI18n passes",
			json: `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","contentI18n":{"en":"Hello","zhTW":"你好"}}]`,
		},
		{
			name:    "unsupported language code rejected",
			json:    `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","contentI18n":{"de":"Hallo"}}]`,
			wantErr: "不支持的语言代码",
		},
		{
			name:    "blank translation rejected",
			json:    `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","contentI18n":{"en":"  "}}]`,
			wantErr: "内容不能为空",
		},
		{
			name:    "non-string translation rejected",
			json:    `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","contentI18n":{"en":123}}]`,
			wantErr: "内容不能为空",
		},
		{
			name:    "non-map contentI18n rejected",
			json:    `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","contentI18n":"en"}]`,
			wantErr: "多语言内容格式错误",
		},
		{
			name:    "extraI18n over 200 bytes rejected",
			json:    `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","extraI18n":{"en":"` + strings.Repeat("x", 201) + `"}}]`,
			wantErr: "语言说明长度不能超过200字符",
		},
		{
			name: "valid extraI18n passes",
			json: `[{"content":"默认","publishDate":"2026-07-01T00:00:00Z","extraI18n":{"ja":"メモ"}}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConsoleSettings(tc.json, "Announcements")
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestGetAnnouncementsPassesThroughContentI18n(t *testing.T) {
	original := GetConsoleSetting().Announcements
	t.Cleanup(func() { GetConsoleSetting().Announcements = original })

	GetConsoleSetting().Announcements = `[
		{"content":"较早","publishDate":"2026-07-01T00:00:00Z"},
		{"content":"较新","publishDate":"2026-07-02T00:00:00Z","contentI18n":{"en":"Newer"},"extraI18n":{"en":"note"}}
	]`

	list := GetAnnouncements()
	require.Len(t, list, 2)
	// 按 publishDate 倒序
	assert.Equal(t, "较新", list[0]["content"])

	i18n, ok := list[0]["contentI18n"].(map[string]interface{})
	require.True(t, ok, "contentI18n should survive as a nested map")
	assert.Equal(t, "Newer", i18n["en"])
	extraI18n, ok := list[0]["extraI18n"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "note", extraI18n["en"])
}
