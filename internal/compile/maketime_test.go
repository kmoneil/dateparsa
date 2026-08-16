package compile

import (
	"testing"
	"time"
)

// makeTime replaces time.Date on the hot path, and the only thing that makes
// that safe is a differential test wide enough to reach the cases nobody thinks
// about. The comparison is == and not Equal: two Times that describe the same
// instant through different fields would pass Equal and would still be a
// difference the caller can see, in Location, in String, and in a map key.
func sameTime(a, b time.Time) bool {
	return a == b && a.Location() == b.Location()
}

func TestMakeTimeMatchesTimeDate(t *testing.T) {
	// The out-of-range values are the point. A day is range-checked against
	// 1..31 with no reference to the month, so February 31st reaches makeTime
	// from a well-formed input; a second is checked against 0..60 for leap
	// seconds; and a base year can put the year anywhere at all.
	years := []int{
		-1, 0, 1, 99, 100, 399, 400, 1582, 1899, 1900, 1901, 1969,
		1970, 1971, 1999, 2000, 2001, 2024, 2100, 2400, 9999, 10000,
	}
	months := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	days := []int{1, 2, 27, 28, 29, 30, 31}
	clocks := []struct{ h, mi, s, ns int }{
		{0, 0, 0, 0},
		{23, 59, 59, 999999999},
		{10, 30, 45, 123000000},
		{12, 0, 60, 0}, // leap second
		{0, 0, 0, 1},
	}

	for _, y := range years {
		for _, mo := range months {
			for _, d := range days {
				for _, c := range clocks {
					want := time.Date(y, time.Month(mo), d, c.h, c.mi, c.s, c.ns, time.UTC)
					got := makeTime(y, time.Month(mo), d, c.h, c.mi, c.s, c.ns, time.UTC)
					if !sameTime(got, want) {
						t.Fatalf("makeTime(%d-%02d-%02d %02d:%02d:%02d.%09d UTC) = %v, time.Date = %v",
							y, mo, d, c.h, c.mi, c.s, c.ns, got, want)
					}
				}
			}
		}
	}
}

// TestMakeTimeMatchesTimeDateEveryDay walks every day of a long span one at a
// time, which is what catches an off-by-one that a grid of round numbers steps
// over: a century rule, a 400-year rule, the March-based year rollover, and the
// day before and after each.
func TestMakeTimeMatchesTimeDateEveryDay(t *testing.T) {
	start := time.Date(1893, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2110, 1, 1, 0, 0, 0, 0, time.UTC)
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		want := time.Date(d.Year(), d.Month(), d.Day(), 6, 7, 8, 9, time.UTC)
		got := makeTime(d.Year(), d.Month(), d.Day(), 6, 7, 8, 9, time.UTC)
		if !sameTime(got, want) {
			t.Fatalf("%v: makeTime = %v, time.Date = %v", d, got, want)
		}
	}
}

// TestMakeTimeMonthOutOfRange covers the arm that folds a month outside 1..12
// into the year. No executor arm can produce one today, since every month op
// range-checks to 1..12, but makeTime is reached by the interpreter's final
// return as well and nothing in its signature says otherwise.
func TestMakeTimeMonthOutOfRange(t *testing.T) {
	for _, mo := range []int{-25, -13, -12, -11, -1, 0, 13, 14, 24, 25, 37} {
		for _, y := range []int{1, 1970, 2024} {
			want := time.Date(y, time.Month(mo), 15, 1, 2, 3, 4, time.UTC)
			got := makeTime(y, time.Month(mo), 15, 1, 2, 3, 4, time.UTC)
			if !sameTime(got, want) {
				t.Errorf("makeTime(%d, month %d) = %v, time.Date = %v", y, mo, got, want)
			}
		}
	}
}

// TestMakeTimeNonUTCDefersToTimeDate is the other half of the contract. A zone
// with transitions cannot be read as an offset, so makeTime must hand those to
// time.Date rather than compute them, and the shortcut must not be reached by a
// Location that merely has a zero offset today.
func TestMakeTimeNonUTCDefersToTimeDate(t *testing.T) {
	locs := []*time.Location{
		time.FixedZone("GMT", 0),
		time.FixedZone("+05:30", 5*3600+30*60),
		time.FixedZone("-08:00", -8*3600),
		time.Local,
	}
	for _, loc := range locs {
		for _, y := range []int{1970, 2024, 1900} {
			want := time.Date(y, 7, 15, 10, 30, 45, 123, loc)
			got := makeTime(y, 7, 15, 10, 30, 45, 123, loc)
			if !sameTime(got, want) {
				t.Errorf("%v %d: makeTime = %v, time.Date = %v", loc, y, got, want)
			}
		}
	}
}

func FuzzMakeTimeMatchesTimeDate(f *testing.F) {
	f.Add(2024, 3, 15, 10, 30, 0, 0)
	f.Add(0, 1, 1, 0, 0, 0, 0)
	f.Add(1970, 1, 1, 0, 0, 0, 0)
	f.Add(2024, 2, 31, 23, 59, 60, 999999999)
	f.Add(-1, 13, 0, 0, 0, 0, 0)
	f.Add(9999, 12, 31, 23, 59, 59, 999999999)

	f.Fuzz(func(t *testing.T, y, mo, d, h, mi, s, ns int) {
		// time.Date itself overflows past these, and no executor arm can reach
		// them: a year comes from four digits or a base time, and the rest are
		// range-checked before they get here.
		if y < -400000 || y > 400000 || mo < -100000 || mo > 100000 ||
			d < -100000 || d > 100000 || h < -100000 || h > 100000 ||
			mi < -100000 || mi > 100000 || s < -100000 || s > 100000 ||
			ns < -1e9 || ns > 1e9 {
			t.Skip()
		}
		want := time.Date(y, time.Month(mo), d, h, mi, s, ns, time.UTC)
		got := makeTime(y, time.Month(mo), d, h, mi, s, ns, time.UTC)
		if !sameTime(got, want) {
			t.Fatalf("makeTime(%d,%d,%d,%d,%d,%d,%d) = %v, time.Date = %v",
				y, mo, d, h, mi, s, ns, got, want)
		}
	})
}
