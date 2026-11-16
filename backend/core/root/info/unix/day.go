package unix

import (
	"fmt"
	"time"
)

// unix seconds of 00:00 UTC on the given calendar date. rejects dates that
// don't exist (e.g. feb 30) instead of letting time.Date roll them over
func DateToDayStart(year, month, day int) (int64, error) {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return 0, fmt.Errorf("%04d-%02d-%02d isn't a real date", year, month, day)
	}
	return t.Unix(), nil
}

// floors any unix timestamp to 00:00 UTC of its day
func FloorToDayStart(ts int64) int64 {
	y, m, d := time.Unix(ts, 0).UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix()
}

// 00:00 UTC today
func TodayStart() int64 {
	return FloorToDayStart(time.Now().Unix())
}
