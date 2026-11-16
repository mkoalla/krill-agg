package krillion

import (
	"testing"
	"time"
)

func TestDayAt(t *testing.T) {
	cases := map[string]int{
		"2026-07-16T04:00:00Z": 1,   // midnight EDT, the first puzzle
		"2026-07-16T03:59:59Z": 0,   // still the day before
		"2026-11-16T12:00:00Z": 124, // the example file's day
		"2026-11-17T04:59:59Z": 124, // 11:59pm EST
		"2026-11-17T05:00:00Z": 125, // midnight EST
		"2027-03-14T04:59:59Z": 241, // DST starts that night; 11:59pm EST
		"2027-03-15T04:00:00Z": 243, // midnight EDT the following day
	}
	for ts, want := range cases {
		at, _ := time.Parse(time.RFC3339, ts)
		if got := DayAt(at); got != want {
			t.Errorf("DayAt(%s) = %d, want %d", ts, got, want)
		}
	}
	for day, want := range map[int]string{1: "2026-07-16T04:00:00Z", 125: "2026-11-17T05:00:00Z", 243: "2027-03-15T04:00:00Z"} {
		if got := StartOf(day).UTC().Format(time.RFC3339); got != want {
			t.Errorf("StartOf(%d) = %s, want %s", day, got, want)
		}
		if got := DayAt(StartOf(day)); got != day {
			t.Errorf("DayAt(StartOf(%d)) = %d", day, got)
		}
	}
	if got := DateOf(124); got != "2026-11-16" {
		t.Errorf("DateOf(124) = %s", got)
	}
	if got := DateOf(1); got != DAY_ONE {
		t.Errorf("DateOf(1) = %s", got)
	}
}
