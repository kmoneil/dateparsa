package dateparsa

import (
	"fmt"
	"testing"
	"time"
)

// coverageCase is one advertised format and an input in it.
//
// ambiguous is what Parse reports for that input, and it is here because
// nothing else checks it. C27 changed what RFC 822 and RFC 850 report, and the
// whole suite stayed green: this test asserted the input parses, oracle_test.go
// compares the instant against time.Parse, and no round-trip spec writes a
// two-digit year beside a month name. A format this file advertises can change
// what it says about its own certainty, which is a change every caller sees,
// and 31 round-trip formats, 23 fuzz targets and the oracle can all be right
// through it.
type coverageCase struct {
	input     string
	dayFirst  bool
	ambiguous bool
	desc      string
}

// coverageCases is the list of formats this library advertises, one input each.
// It is package level rather than local to TestFormatCoverage because
// TestNoCrossFormatDisagreement sweeps every ordered pair of it: a format added
// here is a format that gets checked against all the others for free, which is
// the point of keeping one list.
var coverageCases = []coverageCase{
	// === Structured ISO / RFC ===
	{input: "2024-03-15", desc: "1. ISO date"},
	{input: "2024-03-15T10:30:00", desc: "2. ISO datetime"},
	{input: "2024-03-15T10:30:00Z", desc: "3. ISO datetime UTC"},
	{input: "2024-03-15T10:30:00+05:30", desc: "4. RFC 3339"},
	{input: "2024-03-15T10:30:00-08:00", desc: "5. RFC 3339 negative"},
	{input: "2024-03-15T10:30:00.123456789Z", desc: "6. RFC 3339 nano"},
	{input: "2024-03-15T10:30:00.123+05:30", desc: "7. RFC 3339 frac + tz"},

	// === Text-based RFC formats ===
	{input: "Fri, 15 Mar 2024 10:30:00 +0000", desc: "8. RFC 2822"},
	{input: "Friday, 15-Mar-24 10:30:00 UTC", ambiguous: true, desc: "9. RFC 850"},
	{input: "Fri Mar 15 10:30:00 2024", desc: "10. ANSIC"},
	{input: "Fri Mar 15 10:30:00 UTC 2024", desc: "11. Unix date"},

	// === US formats ===
	{input: "03/15/2024", desc: "12. US slash"},
	{input: "3/15/24", desc: "13. US short"},
	{input: "3/15/2024", desc: "14. US variable width"},
	{input: "March 15, 2024", desc: "15. US long"},
	{input: "Mar 15, 2024", desc: "16. US abbreviated"},

	// === European formats ===
	{input: "15/03/2024", dayFirst: true, desc: "17. European slash (PreferDayFirst)"},
	{input: "15.03.2024", desc: "18. European dot"},
	{input: "15 March 2024", desc: "19. European long"},
	{input: "15-Mar-2024", desc: "20. European dash abbreviated"},

	// === Asian formats ===
	{input: "2024/03/15", desc: "21. Asian slash"},
	{input: "2024.03.15", desc: "22. Asian dot"},

	// === Time only ===
	{input: "10:30", desc: "23. time HH:MM"},
	{input: "10:30:00", desc: "24. time HH:MM:SS"},
	{input: "10:30 PM", desc: "25. time 12h"},
	{input: "10:30:00 PM", desc: "26. time 12h with seconds"},
	{input: "10:30:00.123", desc: "27. time with millis"},
	{input: "10:30:00.123456", desc: "28. time with micros"},

	// === Partial dates ===
	{input: "Mar 15", ambiguous: true, desc: "29. partial month day"},
	{input: "15 Mar", ambiguous: true, desc: "30. partial day month"},
	{input: "March 2024", desc: "31. partial month year"},

	// === Unix timestamps ===
	{input: "1710500000", desc: "32. unix seconds"},
	{input: "1710500000000", desc: "33. unix millis"},
	{input: "1710500000.123", desc: "34. unix fractional"},

	// === Compact formats ===
	{input: "20240315", desc: "35. compact date"},
	{input: "20240315T103000", desc: "36. compact datetime"},
	{input: "20240315103000", desc: "37. compact datetime no sep"},
	{input: "20240315T103000Z", desc: "38. compact datetime UTC"},

	// === ISO week / ordinal ===
	{input: "2024-W11-5", desc: "39. ISO week date"},
	{input: "2024-W11", desc: "40. ISO week"},
	{input: "2024-074", desc: "41. ISO ordinal"},

	// === Syslog / log formats ===
	{input: "Mar 15 10:30:00", ambiguous: true, desc: "42. syslog"},
	{input: "15/Mar/2024:10:30:00 +0000", desc: "43. common log format"},

	// === SQL datetime variants ===
	{input: "2024-03-15 10:30:00", desc: "44. SQL datetime"},
	{input: "2024-03-15 10:30:00.000", desc: "45. SQL millis"},
	{input: "2024-03-15 10:30:00.000000", desc: "46. SQL micros"},
	{input: "2024-03-15 10:30:00+00", desc: "47. SQL short tz"},
	{input: "2024-03-15 10:30:00+05:30", desc: "48. SQL full tz"},

	// === Spreadsheet formats ===
	{input: "3/15/2024 10:30:00 AM", desc: "49. spreadsheet US"},
	{input: "15-Mar-2024 10:30", desc: "50. spreadsheet EU"},

	// === Go reference / RFC 822 / RFC 1123 ===
	{input: "Fri Mar 15 10:30:00 EDT 2024", desc: "51. Go reference time"},
	{input: "15 Mar 24 10:30 UTC", ambiguous: true, desc: "52. RFC 822"},
	{input: "Fri, 15 Mar 2024 10:30:00 UTC", desc: "53. RFC 1123"},
}

func TestFormatCoverage(t *testing.T) {
	tests := coverageCases

	passed := 0
	failed := 0
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			var result ParseResult
			var err error

			if tc.dayFirst {
				result, err = ParseWith(tc.input, WithPreferDayFirst(true))
			} else {
				result, err = Parse(tc.input)
			}

			if err != nil {
				failed++
				t.Errorf("FAIL: %s\n  input:  %q\n  error:  %v", tc.desc, tc.input, err)
				return
			}

			// What the format says about its own certainty is part of what it
			// advertises. Five of these report a guess, and each one is a
			// two-digit number beside a month name that could have been read
			// as a year.
			if result.Ambiguous != tc.ambiguous {
				t.Errorf("%s: Parse(%q).Ambiguous = %v, want %v",
					tc.desc, tc.input, result.Ambiguous, tc.ambiguous)
			}

			passed++
			fmt.Printf("PASS: %-45s -> %v\n", tc.desc, result.Time.Format("2006-01-02 15:04:05.999999999 -0700"))
		})
	}

	t.Logf("\n=== SUMMARY: %d passed, %d failed out of %d total ===", passed, failed, passed+failed)
}

// TestFormatCoverage_Refusals is the half of coverage that the table above
// cannot express: inputs that match a format's shape and name a date that does
// not exist.
//
// It is the assertion that catches C5, and the round-trip specs are not. A
// round trip renders a real time and parses it back, so it only ever produces
// a day-of-year and a week number that exist. Both directions are valid and
// both agree. The bug is on the inputs a generator never generates.
func TestFormatCoverage_Refusals(t *testing.T) {
	tests := []struct {
		input string
		why   string
	}{
		// Day-of-year against the year. 366 exists only in a leap year, and
		// the leap rule is the full one: divisible by 4, except centuries,
		// except centuries divisible by 400.
		{"2023-366", "2023 has 365 days"},
		{"2100-366", "2100 is divisible by 100 and not by 400"},
		{"1900-366", "1900 likewise"},
		{"2023-367", "beyond any year"},
		{"2023-000", "day zero"},

		// Week number against the year. A year has 53 ISO weeks only when
		// 1 January falls on a Thursday, or it is a leap year and 1 January
		// falls on a Wednesday. Every other year has 52.
		{"2023-W53", "2023 began on a Sunday, so it has 52 weeks"},
		{"2021-W53", "2021 began on a Friday"},
		{"2024-W53", "2024 began on a Monday; a leap year is not enough"},
		{"2023-W53-7", "the same week, with a weekday"},
		{"2024-W53-1", "likewise"},
		{"2024-W54", "beyond any year"},
		{"2024-W00", "week zero"},
		{"2024-W11-0", "weekday zero"},
		{"2024-W11-8", "beyond Sunday"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err == nil {
				t.Errorf("Parse(%q) = %v, want an error: %s",
					tt.input, got.Time.UTC().Format("2006-01-02"), tt.why)
			}
		})
	}

	// The neighbours have to keep parsing, or the fix is a blunt refusal
	// rather than a range check.
	for _, tt := range []struct {
		input string
		want  string
	}{
		{"2024-366", "2024-12-31"}, // a leap year does reach 366
		{"2000-366", "2000-12-31"}, // and so does a century divisible by 400
		{"2023-365", "2023-12-31"},
		{"2024-001", "2024-01-01"},
		{"2020-W53", "2020-12-28"}, // 2020 began on a Wednesday and is a leap year
		{"2015-W01", "2014-12-29"}, // ISO week 1 beginning in the previous calendar year
		{"2020-W53-7", "2021-01-03"},
		{"2024-W11-5", "2024-03-15"},
		{"2024-W11", "2024-03-11"},
	} {
		t.Run(tt.input+"_stays", func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}
			if s := got.Time.UTC().Format("2006-01-02"); s != tt.want {
				t.Errorf("Parse(%q) = %s, want %s", tt.input, s, tt.want)
			}
		})
	}
}

// TestOrdinalAndWeekBoundsAgreeWithTheStdlib checks the two predicates behind
// the refusals above against oracles that share no code with them, over every
// year the library is likely to see.
//
// 28 December is always in the last ISO week of its ISO year, so Go's own
// ISOWeek answers how many weeks a year has. time.Parse answers whether a
// day-of-year exists. A hand-written case list would have agreed with a
// hand-written leap rule; these do not.
func TestOrdinalAndWeekBoundsAgreeWithTheStdlib(t *testing.T) {
	for y := 1900; y <= 2100; y++ {
		_, weeks := time.Date(y, 12, 28, 0, 0, 0, 0, time.UTC).ISOWeek()

		last := fmt.Sprintf("%04d-W%02d", y, weeks)
		if _, err := Parse(last); err != nil {
			t.Fatalf("Parse(%q) refused the last week of %d: %v", last, y, err)
		}
		if weeks < 53 {
			over := fmt.Sprintf("%04d-W%02d", y, weeks+1)
			if got, err := Parse(over); err == nil {
				t.Fatalf("Parse(%q) = %s, but %d has %d weeks",
					over, got.Time.UTC().Format("2006-01-02"), y, weeks)
			}
		}

		for _, d := range []int{365, 366} {
			in := fmt.Sprintf("%04d-%03d", y, d)
			_, stdErr := time.Parse("2006-002", in)
			_, ourErr := Parse(in)
			if (stdErr == nil) != (ourErr == nil) {
				t.Fatalf("Parse(%q) err=%v, time.Parse err=%v", in, ourErr, stdErr)
			}
		}
	}
}
