package service

import (
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildInsufficientQuotaMessage 锁定预扣拒绝错误串的信息契约：必须携带模型、分组、
// 分组倍率、上下文估算与「充值/减小上下文」建议，供双前端友好展示与事后取证（预扣 403
// 不落盘，坑点 #138）。钱包/令牌两种归因的主语正确区分。
func TestBuildInsufficientQuotaMessage(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-opus-4-7",
		UsingGroup:      "default",
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 5}},
	}
	info.SetEstimatePromptTokens(200000)

	wallet := buildInsufficientQuotaMessage(info, 4870043, 53350590, false)
	assert.Contains(t, wallet, "用户额度不足")
	assert.Contains(t, wallet, "claude-opus-4-7")
	assert.Contains(t, wallet, "default")
	assert.Contains(t, wallet, "200000")
	assert.Contains(t, wallet, "充值")
	assert.NotContains(t, wallet, "令牌剩余额度")

	token := buildInsufficientQuotaMessage(info, 100, 53350590, true)
	assert.True(t, strings.HasPrefix(token, "令牌额度不足"))
	assert.Contains(t, token, "令牌剩余额度")
}

func TestComputePartialTarget(t *testing.T) {
	tests := []struct {
		name           string
		userQuota      int
		tokenQuota     int
		tokenUnlimited bool
		fullQuota      int
		minQuota       int
		wantTarget     int
		wantReason     preConsumeReject
	}{
		{
			name:       "full covered by wallet and token",
			userQuota:  2000,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 1000,
			wantReason: preConsumeOK,
		},
		{
			name:       "wallet partial covers input min",
			userQuota:  700,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 700,
			wantReason: preConsumeOK,
		},
		{
			name:       "wallet below input min rejects",
			userQuota:  299,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantReason: preConsumeRejectWallet,
		},
		{
			name:       "token partial tightens wallet target",
			userQuota:  900,
			tokenQuota: 500,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 500,
			wantReason: preConsumeOK,
		},
		{
			name:       "token below input min rejects",
			userQuota:  900,
			tokenQuota: 299,
			fullQuota:  1000,
			minQuota:   300,
			wantReason: preConsumeRejectToken,
		},
		{
			name:           "unlimited token ignores token quota",
			userQuota:      700,
			tokenQuota:     0,
			tokenUnlimited: true,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     700,
			wantReason:     preConsumeOK,
		},
		{
			name:       "min equal full preserves hard gate",
			userQuota:  999,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   1000,
			wantReason: preConsumeRejectWallet,
		},
		{
			name:       "zero min allows zero token target",
			userQuota:  500,
			tokenQuota: 0,
			fullQuota:  1000,
			minQuota:   0,
			wantTarget: 0,
			wantReason: preConsumeOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotReason := computePartialTarget(tt.userQuota, tt.tokenQuota, tt.tokenUnlimited, tt.fullQuota, tt.minQuota)
			require.Equal(t, tt.wantReason, gotReason)
			require.Equal(t, tt.wantTarget, gotTarget)
		})
	}
}

func TestResolveFreshTokenTarget(t *testing.T) {
	tests := []struct {
		name           string
		tokenUnlimited bool
		userQuota      int
		fullQuota      int
		minQuota       int
		wantTarget     int
		wantOK         bool
	}{
		{
			name:           "limited token rejects",
			tokenUnlimited: false,
			userQuota:      2000,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     0,
			wantOK:         false,
		},
		{
			name:           "unlimited token full covered",
			tokenUnlimited: true,
			userQuota:      2000,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     1000,
			wantOK:         true,
		},
		{
			name:           "unlimited token wallet partial",
			tokenUnlimited: true,
			userQuota:      700,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     700,
			wantOK:         true,
		},
		{
			name:           "unlimited token wallet below min",
			tokenUnlimited: true,
			userQuota:      299,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     0,
			wantOK:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotOK := resolveFreshTokenTarget(tt.tokenUnlimited, tt.userQuota, tt.fullQuota, tt.minQuota)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantTarget, gotTarget)
		})
	}
}

func TestTokenNonGating(t *testing.T) {
	cases := []struct {
		name           string
		tokenUnlimited bool
		isPlayground   bool
		want           bool
	}{
		{"limited non-playground gates", false, false, false},
		{"unlimited bypasses", true, false, true},
		{"playground bypasses", false, true, true},
		{"unlimited playground bypasses", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenNonGating(tc.tokenUnlimited, tc.isPlayground); got != tc.want {
				t.Fatalf("tokenNonGating(%v,%v)=%v want %v", tc.tokenUnlimited, tc.isPlayground, got, tc.want)
			}
		})
	}
}
