package krillion

import (
	"time"
	_ "time/tzdata" // the runtime image has no zoneinfo
)

// puzzles roll over at midnight US eastern (DST-aware)
var RESET_ZONE = mustZone("America/New_York")

// krillion #1, in RESET_ZONE. every later puzzle is one calendar day on
const DAY_ONE = "2026-07-16"

var dayOne = mustDate(DAY_ONE)

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func mustDate(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// the puzzle that's live at t
func DayAt(t time.Time) int {
	y, m, d := t.In(RESET_ZONE).Date()
	// count calendar days on the date alone, so DST's 23/25h days don't skew it
	local := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int(local.Sub(dayOne).Hours()/24) + 1
}

func Today() int {
	return DayAt(time.Now())
}

// the moment a puzzle goes live: midnight in RESET_ZONE on its date
func StartOf(day int) time.Time {
	y, m, d := dayOne.AddDate(0, 0, day-1).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, RESET_ZONE)
}

// "2026-11-16" for #124, the date the krillion api uses
func DateOf(day int) string {
	return dayOne.AddDate(0, 0, day-1).Format(time.DateOnly)
}
