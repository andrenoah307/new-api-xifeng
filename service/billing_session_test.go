package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
