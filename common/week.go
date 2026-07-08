package common

import "time"

var cstZone = time.FixedZone("CST", 8*3600)

// WeekStartUnixUTC8 返回 nowUnix 所在自然周的周一 00:00:00 (UTC+8) 的 unix 秒。
func WeekStartUnixUTC8(nowUnix int64) int64 {
	t := time.Unix(nowUnix, 0).In(cstZone)
	weekday := int(t.Weekday()) // Sunday=0..Saturday=6
	if weekday == 0 {
		weekday = 7
	}
	monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, cstZone).AddDate(0, 0, -(weekday - 1))
	return monday.Unix()
}

// WeekEndUnixUTC8 返回该自然周的下周一 00:00:00 (UTC+8)（重置点 / 独占上界）。
func WeekEndUnixUTC8(nowUnix int64) int64 {
	return WeekStartUnixUTC8(nowUnix) + 7*24*3600
}
