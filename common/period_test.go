package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var periodTestUTC8 = time.FixedZone("UTC+8", 8*60*60)

func TestTokenPeriodBoundsDaysUsesAnchorAtCalendarMidnights(t *testing.T) {
	anchor := time.Date(2026, 1, 30, 0, 0, 0, 0, periodTestUTC8)
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "at anchor",
			now:  time.Date(2026, 1, 30, 0, 0, 0, 0, periodTestUTC8),
			want: time.Date(2026, 1, 30, 0, 0, 0, 0, periodTestUTC8),
		},
		{
			name: "one second before next bucket",
			now:  time.Date(2026, 2, 1, 23, 59, 59, 0, periodTestUTC8),
			want: time.Date(2026, 1, 30, 0, 0, 0, 0, periodTestUTC8),
		},
		{
			name: "exactly on N day boundary",
			now:  time.Date(2026, 2, 2, 0, 0, 0, 0, periodTestUTC8),
			want: time.Date(2026, 2, 2, 0, 0, 0, 0, periodTestUTC8),
		},
		{
			name: "crosses a year",
			now:  time.Date(2027, 1, 1, 12, 0, 0, 0, periodTestUTC8),
			want: time.Date(2027, 1, 1, 0, 0, 0, 0, periodTestUTC8),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := TokenPeriodBounds(TokenPeriodTypeDays, 3, anchor.Unix(), tt.now)
			require.True(t, ok)
			assert.Equal(t, tt.want.Unix(), start)
			assert.Equal(t, tt.want.AddDate(0, 0, 3).Unix(), end)
		})
	}
}

func TestTokenPeriodBoundsDaysSupportsExtremeLengthsAndFutureAnchors(t *testing.T) {
	tests := []struct {
		name       string
		periodDays int
		anchor     time.Time
		now        time.Time
		wantStart  time.Time
		wantEnd    time.Time
	}{
		{
			name:       "one day",
			periodDays: 1,
			anchor:     time.Date(2026, 6, 1, 0, 0, 0, 0, periodTestUTC8),
			now:        time.Date(2026, 6, 2, 23, 59, 59, 0, periodTestUTC8),
			wantStart:  time.Date(2026, 6, 2, 0, 0, 0, 0, periodTestUTC8),
			wantEnd:    time.Date(2026, 6, 3, 0, 0, 0, 0, periodTestUTC8),
		},
		{
			name:       "3650 days",
			periodDays: 3650,
			anchor:     time.Date(2020, 1, 1, 0, 0, 0, 0, periodTestUTC8),
			now:        time.Date(2026, 8, 2, 18, 0, 0, 0, periodTestUTC8),
			wantStart:  time.Date(2020, 1, 1, 0, 0, 0, 0, periodTestUTC8),
			wantEnd:    time.Date(2029, 12, 29, 0, 0, 0, 0, periodTestUTC8),
		},
		{
			name:       "future anchor falls back to today",
			periodDays: 3,
			anchor:     time.Date(2026, 8, 3, 0, 0, 0, 0, periodTestUTC8),
			now:        time.Date(2026, 8, 2, 12, 0, 0, 0, periodTestUTC8),
			wantStart:  time.Date(2026, 8, 2, 0, 0, 0, 0, periodTestUTC8),
			wantEnd:    time.Date(2026, 8, 5, 0, 0, 0, 0, periodTestUTC8),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := TokenPeriodBounds(TokenPeriodTypeDays, tt.periodDays, tt.anchor.Unix(), tt.now)
			require.True(t, ok)
			assert.Equal(t, tt.wantStart.Unix(), start)
			assert.Equal(t, tt.wantEnd.Unix(), end)
		})
	}
}

func TestTokenPeriodBoundsWeekAndMonth(t *testing.T) {
	weekMonday := time.Date(2026, 8, 3, 0, 0, 0, 0, periodTestUTC8)
	weekSunday := time.Date(2026, 8, 9, 23, 59, 59, 0, periodTestUTC8)
	for _, now := range []time.Time{weekMonday, weekSunday} {
		start, end, ok := TokenPeriodBounds(TokenPeriodTypeWeek, 0, 0, now)
		require.True(t, ok)
		assert.Equal(t, weekMonday.Unix(), start)
		assert.Equal(t, weekMonday.AddDate(0, 0, 7).Unix(), end)
	}

	monthCases := []struct {
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{time.Date(2026, 2, 1, 0, 0, 0, 0, periodTestUTC8), time.Date(2026, 2, 1, 0, 0, 0, 0, periodTestUTC8), time.Date(2026, 3, 1, 0, 0, 0, 0, periodTestUTC8)},
		{time.Date(2024, 2, 29, 23, 59, 59, 0, periodTestUTC8), time.Date(2024, 2, 1, 0, 0, 0, 0, periodTestUTC8), time.Date(2024, 3, 1, 0, 0, 0, 0, periodTestUTC8)},
		{time.Date(2026, 4, 30, 12, 0, 0, 0, periodTestUTC8), time.Date(2026, 4, 1, 0, 0, 0, 0, periodTestUTC8), time.Date(2026, 5, 1, 0, 0, 0, 0, periodTestUTC8)},
		{time.Date(2026, 12, 31, 23, 59, 59, 0, periodTestUTC8), time.Date(2026, 12, 1, 0, 0, 0, 0, periodTestUTC8), time.Date(2027, 1, 1, 0, 0, 0, 0, periodTestUTC8)},
	}
	for _, tt := range monthCases {
		start, end, ok := TokenPeriodBounds(TokenPeriodTypeMonth, 0, 0, tt.now)
		require.True(t, ok)
		assert.Equal(t, tt.wantStart.Unix(), start)
		assert.Equal(t, tt.wantEnd.Unix(), end)
	}
}

func TestTokenPeriodBoundsRejectsInvalidDaysConfiguration(t *testing.T) {
	for _, periodType := range []string{"", "year", "DAYS"} {
		start, end, ok := TokenPeriodBounds(periodType, 1, 0, time.Now())
		require.False(t, ok)
		assert.Zero(t, start)
		assert.Zero(t, end)
	}
	for _, periodDays := range []int{0, -1} {
		start, end, ok := TokenPeriodBounds(TokenPeriodTypeDays, periodDays, 0, time.Now())
		require.False(t, ok)
		assert.Zero(t, start)
		assert.Zero(t, end)
	}
}

func TestTokenPeriodBoundsDoesNotDependOnInputOrHostLocation(t *testing.T) {
	utcNow := time.Date(2026, 8, 2, 15, 30, 0, 0, time.UTC)
	newYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	nyNow := utcNow.In(newYork)
	anchor := time.Date(2026, 7, 31, 0, 0, 0, 0, periodTestUTC8).Unix()

	startUTC, endUTC, ok := TokenPeriodBounds(TokenPeriodTypeDays, 3, anchor, utcNow)
	require.True(t, ok)
	startNY, endNY, ok := TokenPeriodBounds(TokenPeriodTypeDays, 3, anchor, nyNow)
	require.True(t, ok)
	assert.Equal(t, startUTC, startNY)
	assert.Equal(t, endUTC, endNY)

	for _, tz := range []string{"UTC", "America/New_York"} {
		t.Setenv("TZ", tz)
		assert.Equal(t, TokenPeriodAnchorNow(utcNow), TokenPeriodAnchorNow(nyNow))
	}
}

func TestTokenPeriodAnchorNowUsesUTC8Midnight(t *testing.T) {
	now := time.Date(2026, 8, 2, 23, 59, 59, 0, time.UTC)
	want := time.Date(2026, 8, 3, 0, 0, 0, 0, periodTestUTC8)
	assert.Equal(t, want.Unix(), TokenPeriodAnchorNow(now))
}
