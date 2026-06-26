package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeekStartUnixUTC8(t *testing.T) {
	tests := []struct {
		name      string
		nowUnix   int64
		weekStart int64
	}{
		{
			name:      "wednesday noon utc8",
			nowUnix:   1704254400,
			weekStart: 1704038400,
		},
		{
			name:      "monday boundary",
			nowUnix:   1704038400,
			weekStart: 1704038400,
		},
		{
			name:      "sunday end utc8",
			nowUnix:   1704643199,
			weekStart: 1704038400,
		},
		{
			name:      "next monday boundary",
			nowUnix:   1704643200,
			weekStart: 1704643200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.weekStart, WeekStartUnixUTC8(tt.nowUnix))
		})
	}
}

func TestWeekEndUnixUTC8(t *testing.T) {
	tests := []struct {
		name    string
		nowUnix int64
		want    int64
	}{
		{
			name:    "wednesday noon utc8",
			nowUnix: 1704254400,
			want:    1704643200,
		},
		{
			name:    "monday boundary",
			nowUnix: 1704038400,
			want:    1704643200,
		},
		{
			name:    "sunday end utc8",
			nowUnix: 1704643199,
			want:    1704643200,
		},
		{
			name:    "next monday boundary",
			nowUnix: 1704643200,
			want:    1705248000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WeekEndUnixUTC8(tt.nowUnix))
		})
	}
}

func TestWeekEndUnixUTC8Invariant(t *testing.T) {
	samples := []int64{
		1704038400,
		1704254400,
		1704643199,
		1704643200,
		1705248000,
	}

	for _, nowUnix := range samples {
		assert.Equal(t, int64(604800), WeekEndUnixUTC8(nowUnix)-WeekStartUnixUTC8(nowUnix))
	}
}
