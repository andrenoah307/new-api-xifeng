package common

import "time"

const (
	TokenPeriodTypeDays  = "days"
	TokenPeriodTypeWeek  = "week"
	TokenPeriodTypeMonth = "month"
)

const tokenPeriodDaySeconds int64 = 24 * 60 * 60

// TokenPeriodBounds returns the half-open [start, end) bucket containing now.
// All calendar calculations use the application's fixed UTC+8 zone; the
// database never participates in date arithmetic.
func TokenPeriodBounds(periodType string, periodDays int, anchorAt int64, now time.Time) (start int64, end int64, ok bool) {
	localNow := now.In(cstZone)
	switch periodType {
	case TokenPeriodTypeDays:
		if periodDays <= 0 {
			return 0, 0, false
		}
		todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, cstZone).Unix()
		anchor := anchorAt
		if anchor <= 0 || anchor > todayStart {
			anchor = todayStart
		}
		bucketSeconds := int64(periodDays) * tokenPeriodDaySeconds
		bucketIndex := (todayStart - anchor) / bucketSeconds
		start = anchor + bucketIndex*bucketSeconds
		return start, start + bucketSeconds, true

	case TokenPeriodTypeWeek:
		start = WeekStartUnixUTC8(localNow.Unix())
		return start, WeekEndUnixUTC8(localNow.Unix()), true

	case TokenPeriodTypeMonth:
		monthStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, cstZone)
		return monthStart.Unix(), monthStart.AddDate(0, 1, 0).Unix(), true

	default:
		return 0, 0, false
	}
}

// TokenPeriodAnchorNow returns the UTC+8 midnight at the start of now's
// calendar day. It is persisted when a token period policy is enabled or
// when its bucket shape changes.
func TokenPeriodAnchorNow(now time.Time) int64 {
	localNow := now.In(cstZone)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, cstZone).Unix()
}
