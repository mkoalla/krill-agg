package unix

import "testing"

func TestDayStart(t *testing.T) {
	ds, err := DateToDayStart(2026, 11, 16)
	if err != nil || ds != 1794787200 {
		t.Fatalf("DateToDayStart(2026-11-16) = %d %v, want 1794787200", ds, err)
	}
	if _, err := DateToDayStart(2026, 2, 30); err == nil {
		t.Error("feb 30 accepted")
	}
	if _, err := DateToDayStart(2026, 13, 1); err == nil {
		t.Error("month 13 accepted")
	}
	for _, ts := range []int64{1794787200, 1794787200 + 1, 1794787200 + 86399} {
		if got := FloorToDayStart(ts); got != 1794787200 {
			t.Errorf("FloorToDayStart(%d) = %d", ts, got)
		}
	}
	if got := FloorToDayStart(-1); got != -86400 {
		t.Errorf("FloorToDayStart(-1) = %d, want -86400", got)
	}
}
