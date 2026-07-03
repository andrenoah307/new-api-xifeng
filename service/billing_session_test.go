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
		wantOK         bool
	}{
		{
			name:       "full covered by wallet and token",
			userQuota:  2000,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 1000,
			wantOK:     true,
		},
		{
			name:       "wallet partial covers input min",
			userQuota:  700,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 700,
			wantOK:     true,
		},
		{
			name:       "wallet below input min rejects",
			userQuota:  299,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   300,
			wantOK:     false,
		},
		{
			name:       "token partial tightens wallet target",
			userQuota:  900,
			tokenQuota: 500,
			fullQuota:  1000,
			minQuota:   300,
			wantTarget: 500,
			wantOK:     true,
		},
		{
			name:       "token below input min rejects",
			userQuota:  900,
			tokenQuota: 299,
			fullQuota:  1000,
			minQuota:   300,
			wantOK:     false,
		},
		{
			name:           "unlimited token ignores token quota",
			userQuota:      700,
			tokenQuota:     0,
			tokenUnlimited: true,
			fullQuota:      1000,
			minQuota:       300,
			wantTarget:     700,
			wantOK:         true,
		},
		{
			name:       "min equal full preserves hard gate",
			userQuota:  999,
			tokenQuota: 2000,
			fullQuota:  1000,
			minQuota:   1000,
			wantOK:     false,
		},
		{
			name:       "zero min allows zero token target",
			userQuota:  500,
			tokenQuota: 0,
			fullQuota:  1000,
			minQuota:   0,
			wantTarget: 0,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotOK := computePartialTarget(tt.userQuota, tt.tokenQuota, tt.tokenUnlimited, tt.fullQuota, tt.minQuota)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantTarget, gotTarget)
		})
	}
}
