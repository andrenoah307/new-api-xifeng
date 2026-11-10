package setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketQuickRepliesNormalization(t *testing.T) {
	got, err := NormalizeTicketQuickReplies("  First reply \r\n\nSecond reply\nFirst reply\n")
	require.NoError(t, err)
	assert.Equal(t, "First reply\nSecond reply", got)
	got, err = NormalizeTicketQuickReplies(" \n")
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = NormalizeTicketQuickReplies(strings.Repeat("x", 5001))
	require.Error(t, err)
	lines := make([]string, 51)
	for i := range lines {
		lines[i] = strings.Repeat("x", i+1)
	}
	_, err = NormalizeTicketQuickReplies(strings.Join(lines, "\n"))
	require.Error(t, err)
}

func TestCustomNavigationLinksValidation(t *testing.T) {
	valid := CustomNavigationLink{ID: "business", Title: " Business ", URL: "https://example.com/form", Mode: "iframe", Header: true, Sidebar: true}
	tests := []struct {
		name   string
		change func(*CustomNavigationLink)
	}{
		{"script URL", func(l *CustomNavigationLink) { l.URL = "javascript:alert(1)" }},
		{"relative URL", func(l *CustomNavigationLink) { l.URL = "//example.com/form" }},
		{"credentials", func(l *CustomNavigationLink) { l.URL = "https://user:secret@example.com" }},
		{"insecure frame", func(l *CustomNavigationLink) { l.URL = "http://example.com" }},
		{"invalid mode", func(l *CustomNavigationLink) { l.Mode = "script" }},
		{"empty title", func(l *CustomNavigationLink) { l.Title = " " }},
		{"route injection", func(l *CustomNavigationLink) { l.ID = "../admin" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			link := valid
			tc.change(&link)
			raw, err := common.Marshal([]CustomNavigationLink{link})
			require.NoError(t, err)
			_, err = NormalizeCustomNavigationLinks(string(raw))
			require.Error(t, err)
		})
	}
	second := valid
	second.ID, second.Mode, second.URL = "docs", "external", "http://example.com/docs"
	raw, err := common.Marshal([]CustomNavigationLink{valid, second})
	require.NoError(t, err)
	normalized, err := NormalizeCustomNavigationLinks(string(raw))
	require.NoError(t, err)
	var result []CustomNavigationLink
	require.NoError(t, common.UnmarshalJsonStr(normalized, &result))
	valid.Title = "Business"
	second.Title = "Business"
	assert.Equal(t, []CustomNavigationLink{valid, second}, result)
	raw, err = common.Marshal([]CustomNavigationLink{valid, valid})
	require.NoError(t, err)
	_, err = NormalizeCustomNavigationLinks(string(raw))
	require.Error(t, err)
	for _, empty := range []string{"", "[]", "null"} {
		got, err := NormalizeCustomNavigationLinks(empty)
		require.NoError(t, err)
		assert.Equal(t, "[]", got)
	}
}
