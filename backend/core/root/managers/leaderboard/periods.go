package leaderboard

import (
	"krill-agg/root/krillion"
	"time"
)

// how far back a board looks
const (
	WEEK    = "week"
	MONTH   = "month"
	MONTHS3 = "3months"
	MONTHS6 = "6months"
	YTD     = "ytd"
	ALLTIME = "all"
)

// the order /top offers them in
var PERIODS = []string{WEEK, MONTH, MONTHS3, MONTHS6, YTD, ALLTIME}

var PERIOD_LABELS = map[string]string{
	WEEK:    "1 Week",
	MONTH:   "1 Month",
	MONTHS3: "3 Months",
	MONTHS6: "6 Months",
	YTD:     "YTD",
	ALLTIME: "All-time",
}

// the first krillion # a period counts, as of today's. calendar spans in
// krillion's own zone that end with today: a week is today and the 6 days
// before, a month runs from the day after this date last month, YTD from
// january 1. never before #1; unknown periods count everything
func MinDay(period string, today int) int {
	start := krillion.StartOf(today)
	var from int
	switch period {
	case WEEK:
		from = today - 6
	case MONTH:
		from = krillion.DayAt(start.AddDate(0, -1, 0)) + 1
	case MONTHS3:
		from = krillion.DayAt(start.AddDate(0, -3, 0)) + 1
	case MONTHS6:
		from = krillion.DayAt(start.AddDate(0, -6, 0)) + 1
	case YTD:
		year := start.In(krillion.RESET_ZONE).Year()
		from = krillion.DayAt(time.Date(year, time.January, 1, 12, 0, 0, 0, krillion.RESET_ZONE))
	default:
		from = 1
	}
	return max(from, 1)
}
