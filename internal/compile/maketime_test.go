package compile

import (
	"fmt"
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
					got := makeTime(y, time.Month(mo), d, c.h, c.mi, c.s, c.ns, time.UTC, 0)
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
		got := makeTime(d.Year(), d.Month(), d.Day(), 6, 7, 8, 9, time.UTC, 0)
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
			got := makeTime(y, time.Month(mo), 15, 1, 2, 3, 4, time.UTC, 0)
			if !sameTime(got, want) {
				t.Errorf("makeTime(%d, month %d) = %v, time.Date = %v", y, mo, got, want)
			}
		}
	}
}

// TestMakeTimeNonUTCDefersToTimeDate is the other half of the contract. A zone
// with transitions cannot be read as an offset, so makeTime must hand those to
// time.Date rather than compute them, and the shortcut must not be reached by a
// Location whose offset this package did not itself compute.
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
			got := makeTime(y, 7, 15, 10, 30, 45, 123, loc, zoneOffsetUnknown)
			if !sameTime(got, want) {
				t.Errorf("%v %d: makeTime = %v, time.Date = %v", loc, y, got, want)
			}
		}
	}
}

// TestMakeTimeKnownOffsetMatchesTimeDate is the other half, and it is the one
// P19 added: where the offset IS known, the arithmetic has to answer exactly
// what time.Date answers, name and all.
//
// The zone name matters and is why sameTime is not enough on its own here.
// time.Format("MST") prints it, so a Location reached by a different route
// would be visible to a caller even if the instant were right.
func TestMakeTimeKnownOffsetMatchesTimeDate(t *testing.T) {
	type zone struct {
		loc *time.Location
		off int
	}
	zones := []zone{
		{time.UTC, 0},
		{time.FixedZone("+05:30", 5*3600+30*60), 5*3600 + 30*60},
		{time.FixedZone("-08:00", -8*3600), -8 * 3600},
		{time.FixedZone("+14:00", 14*3600), 14 * 3600},
		{time.FixedZone("-00:44", -44*60), -44 * 60},
		{time.FixedZone("GMT", 0), 0},
	}
	for _, z := range zones {
		for _, y := range []int{1900, 1970, 1999, 2000, 2024, 2100, 2399} {
			for _, mo := range []int{1, 2, 3, 12} {
				for _, d := range []int{1, 28, 29, 31} {
					want := time.Date(y, time.Month(mo), d, 10, 30, 45, 123, z.loc)
					got := makeTime(y, time.Month(mo), d, 10, 30, 45, 123, z.loc, z.off)
					if !sameTime(got, want) {
						t.Fatalf("%v %d-%02d-%02d: makeTime = %v, time.Date = %v",
							z.loc, y, mo, d, got, want)
					}
					if got.Format("MST") != want.Format("MST") {
						t.Fatalf("%v: zone name %q, want %q", z.loc, got.Format("MST"), want.Format("MST"))
					}
				}
			}
		}
	}
}

// TestZoneLookupsReportTheirOffset holds the two producers to the offset they
// report, because makeTime now trusts that number instead of asking the
// Location. A wrong one here is an instant off by hours with a nil error, which
// is the failure P19 is one mistake away from.
func TestZoneLookupsReportTheirOffset(t *testing.T) {
	for _, name := range []string{
		"UTC", "UCT", "GMT", "EST", "EDT", "CST", "CDT", "MST", "MDT",
		"PST", "PDT", "HST", "CET", "EET", "MET", "WET",
	} {
		loc, off, ok := lookupTZAbbr(name)
		if !ok {
			t.Errorf("%s: not found", name)
			continue
		}
		_, want := time.Date(2024, 6, 15, 12, 0, 0, 0, loc).Zone()
		if off != want {
			t.Errorf("%s: reports %d, the Location says %d", name, off, want)
		}
	}

	// Every offset the numeric parser accepts, at both spellings and at the
	// three widths, on and off the fifteen-minute grid.
	for h := 0; h <= 23; h++ {
		for _, m := range []int{0, 15, 30, 44, 45, 53, 59} {
			for _, sign := range []string{"+", "-"} {
				in := fmt.Sprintf("%s%02d:%02d", sign, h, m)
				loc, off, ok := parseTZOffset(in, 0, len(in))
				if !ok {
					t.Errorf("%s: refused", in)
					continue
				}
				_, want := time.Date(2024, 6, 15, 12, 0, 0, 0, loc).Zone()
				if off != want {
					t.Errorf("%s: reports %d, the Location says %d", in, off, want)
				}
			}
		}
	}
}

// FuzzZoneOffsetMatchesItsLocation fuzzes the same promise: whatever
// parseTZOffset accepts, the offset it reports is the offset its Location
// applies.
func FuzzZoneOffsetMatchesItsLocation(f *testing.F) {
	for _, s := range []string{"+05:30", "-0800", "+00", "+14:00", "-23:59", "+0000", "Z", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8 {
			t.Skip()
		}
		loc, off, ok := parseTZOffset(s, 0, len(s))
		if !ok {
			return
		}
		_, want := time.Date(2024, 6, 15, 12, 0, 0, 0, loc).Zone()
		if off != want {
			t.Fatalf("%q: reports %d, the Location applies %d", s, off, want)
		}
		// And the whole point: makeTime must agree with time.Date through it.
		a := makeTime(2024, 6, 15, 12, 0, 0, 0, loc, off)
		b := time.Date(2024, 6, 15, 12, 0, 0, 0, loc)
		if !sameTime(a, b) {
			t.Fatalf("%q: makeTime %v, time.Date %v", s, a, b)
		}
	})
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
		got := makeTime(y, time.Month(mo), d, h, mi, s, ns, time.UTC, 0)
		if !sameTime(got, want) {
			t.Fatalf("makeTime(%d,%d,%d,%d,%d,%d,%d) = %v, time.Date = %v",
				y, mo, d, h, mi, s, ns, got, want)
		}
	})
}

// TestDayTableMatchesExact is the whole of P20's correctness argument: the
// table and Hinnant's arithmetic answer the same for every date the table
// covers, and daysFromCivil hands anything else to the exact function.
//
// It walks every year in the table and every month, plus the day boundaries
// that decide leap handling, and then walks past both ends so the fall-through
// is exercised rather than assumed. Roughly 600 * 12 * 8 comparisons, which is
// a tenth of a second and worth it: the failure this prevents is a date off by
// one day, returned with a nil error.
func TestDayTableMatchesExact(t *testing.T) {
	days := []int{-1, 0, 1, 27, 28, 29, 30, 31, 32}
	checked := 0
	for y := dayTableLoYear - 3; y < dayTableHiYear+3; y++ {
		for m := 1; m <= 12; m++ {
			for _, d := range days {
				got := daysFromCivil(y, m, d)
				want := daysFromCivilExact(y, m, d)
				if got != want {
					t.Fatalf("daysFromCivil(%d,%d,%d) = %d, exact = %d", y, m, d, got, want)
				}
				checked++
			}
		}
	}
	if checked < 50000 {
		t.Fatalf("only %d combinations checked; the table range shrank", checked)
	}

	// The months the table cannot index, which have to fall through. A month of
	// 13 is January of the next year and a month of 0 is December of the last,
	// the way time.Date reads them.
	for _, m := range []int{-13, -1, 0, 13, 25, 100000} {
		for _, y := range []int{1970, 2024, dayTableLoYear, dayTableHiYear - 1} {
			if got, want := daysFromCivil(y, m, 15), daysFromCivilExact(y, m, 15); got != want {
				t.Errorf("daysFromCivil(%d,%d,15) = %d, exact = %d", y, m, got, want)
			}
		}
	}
}

// TestDayTableCoversTheYearsThatParse checks the table actually spans what a
// four-digit year field can produce in practice, so the fast arm is the one a
// real timestamp takes rather than a branch nothing reaches.
func TestDayTableCoversTheYearsThatParse(t *testing.T) {
	for _, y := range []int{1900, 1970, 1999, 2000, 2024, 2038, 2100, 2200} {
		if uint(y-dayTableLoYear) >= dayTableSize {
			t.Errorf("year %d is outside the table and is a year callers parse", y)
		}
	}
	// And the two-digit year pivot's ends, which NormalizeTwoDigitYear produces.
	for _, y := range []int{NormalizeTwoDigitYear(0), NormalizeTwoDigitYear(68), NormalizeTwoDigitYear(69), NormalizeTwoDigitYear(99)} {
		if uint(y-dayTableLoYear) >= dayTableSize {
			t.Errorf("two-digit pivot produces %d, outside the table", y)
		}
	}
}

// FuzzDaysFromCivilMatchesExact fuzzes the same property the table test asserts
// on a grid, over whatever the corpus reaches, including the fall-through.
func FuzzDaysFromCivilMatchesExact(f *testing.F) {
	f.Add(2024, 3, 15)
	f.Add(1799, 12, 31)
	f.Add(2400, 1, 1)
	f.Add(0, 1, 1)
	f.Add(-1, 13, 0)
	f.Add(9999, 12, 31)
	f.Fuzz(func(t *testing.T, y, m, d int) {
		// The same bounds FuzzMakeTimeMatchesTimeDate uses, for the same
		// reason: past them the int64 day count overflows and time.Date does
		// not agree with anything either.
		if y < -400000 || y > 400000 || m < -100000 || m > 100000 ||
			d < -100000 || d > 100000 {
			t.Skip()
		}
		if got, want := daysFromCivil(y, m, d), daysFromCivilExact(y, m, d); got != want {
			t.Fatalf("daysFromCivil(%d,%d,%d) = %d, exact = %d", y, m, d, got, want)
		}
	})
}
