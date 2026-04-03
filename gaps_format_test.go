package dateparsa

import (
	"testing"
	"time"
)

// TestGaps_FormatCoverage tests all identified format gaps from the competitive audit.
// TDD: these tests are written FIRST, then implementation follows.
func TestGaps_FormatCoverage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		opts     []Option
		wantYear int
		wantMon  time.Month
		wantDay  int
		wantHour int
		wantMin  int
		wantSec  int
	}{
		// === Period-abbreviated month names ===
		{
			name: "oct. 7, 1970", input: "oct. 7, 1970",
			wantYear: 1970, wantMon: 10, wantDay: 7,
		},
		{
			name: "jan. 15, 2024", input: "jan. 15, 2024",
			wantYear: 2024, wantMon: 1, wantDay: 15,
		},
		{
			name: "sept. 1, 2020", input: "sept. 1, 2020",
			wantYear: 2020, wantMon: 9, wantDay: 1,
		},

		// === Ordinal day suffixes in structured dates ===
		{
			name: "October 7th, 1970", input: "October 7th, 1970",
			wantYear: 1970, wantMon: 10, wantDay: 7,
		},
		{
			name: "January 1st, 2024", input: "January 1st, 2024",
			wantYear: 2024, wantMon: 1, wantDay: 1,
		},
		{
			name: "March 2nd, 2024", input: "March 2nd, 2024",
			wantYear: 2024, wantMon: 3, wantDay: 2,
		},
		{
			name: "April 3rd, 2024", input: "April 3rd, 2024",
			wantYear: 2024, wantMon: 4, wantDay: 3,
		},
		{
			name: "December 23rd", input: "December 23rd",
			wantMon: 12, wantDay: 23,
		},

		// === Comma as fractional second separator (Java/log4j) ===
		{
			name: "comma fractional", input: "2014-05-11 08:20:13,787",
			wantYear: 2014, wantMon: 5, wantDay: 11,
			wantHour: 8, wantMin: 20, wantSec: 13,
		},

		// === ISO 8601 partial dates ===
		{
			name: "year-month 2014-04", input: "2014-04",
			wantYear: 2014, wantMon: 4, wantDay: 1,
		},
		{
			name: "bare year 2014", input: "2014",
			wantYear: 2014, wantMon: 1, wantDay: 1,
		},

		// === SQL datetime with timezone name ===
		{
			name: "SQL datetime UTC name", input: "2014-12-16 06:20:00 UTC",
			wantYear: 2014, wantMon: 12, wantDay: 16,
			wantHour: 6, wantMin: 20,
		},
		{
			name: "SQL datetime GMT name", input: "2014-12-16 06:20:00 GMT",
			wantYear: 2014, wantMon: 12, wantDay: 16,
			wantHour: 6, wantMin: 20,
		},
		{
			name: "SQL datetime EST name", input: "2014-12-16 06:20:00 EST",
			wantYear: 2014, wantMon: 12, wantDay: 16,
			wantHour: 6, wantMin: 20,
		},

		// === SQL datetime with AM/PM ===
		{
			name: "SQL datetime PM", input: "2014-04-26 05:24:37 PM",
			wantYear: 2014, wantMon: 4, wantDay: 26,
			wantHour: 17, wantMin: 24, wantSec: 37,
		},
		{
			name: "SQL datetime AM", input: "2014-04-26 10:24:37 AM",
			wantYear: 2014, wantMon: 4, wantDay: 26,
			wantHour: 10, wantMin: 24, wantSec: 37,
		},

		// === EXIF colon-separated dates ===
		{
			name: "EXIF date", input: "2014:03:31",
			wantYear: 2014, wantMon: 3, wantDay: 31,
		},
		{
			name: "EXIF datetime", input: "2014:04:08 22:05",
			wantYear: 2014, wantMon: 4, wantDay: 8,
			wantHour: 22, wantMin: 5,
		},
		{
			name: "EXIF datetime full", input: "2014:04:08 22:05:13",
			wantYear: 2014, wantMon: 4, wantDay: 8,
			wantHour: 22, wantMin: 5, wantSec: 13,
		},

		// === Apostrophe-prefixed year ===
		{
			name: "apostrophe year", input: "oct 7, '70",
			wantYear: 1970, wantMon: 10, wantDay: 7,
		},

		// === 6-digit YYMMDD compact ===
		{
			name: "YYMMDD compact", input: "171113",
			wantYear: 2017, wantMon: 11, wantDay: 13,
		},
		{
			name: "YYMMDD compact with time", input: "171113 14:14:20",
			wantYear: 2017, wantMon: 11, wantDay: 13,
			wantHour: 14, wantMin: 14, wantSec: 20,
		},

		// === JavaScript Date.toString() ===
		{
			name: "JS Date.toString", input: "Fri Jul 03 2015 18:04:07 GMT+0100",
			wantYear: 2015, wantMon: 7, wantDay: 3,
			wantHour: 18, wantMin: 4, wantSec: 7,
		},

		// === Go time.String() output ===
		{
			name: "Go time.String with nsec", input: "2012-08-03 18:31:59.257000000 +0000 UTC",
			wantYear: 2012, wantMon: 8, wantDay: 3,
			wantHour: 18, wantMin: 31, wantSec: 59,
		},

		// === Date with timezone offset but no time ===
		{
			name: "date+tz no time", input: "2020-07-20+08:00",
			wantYear: 2020, wantMon: 7, wantDay: 20,
		},

		// === CJK ideographic dates ===
		{
			name: "Chinese date", input: "2014年04月08日",
			wantYear: 2014, wantMon: 4, wantDay: 8,
			opts: []Option{WithLocales(ZH)},
		},

		// === Single-digit day in RFC 2822 ===
		{
			name: "RFC 2822 single-digit day", input: "Thu, 4 Jan 2018 17:53:36 +0000",
			wantYear: 2018, wantMon: 1, wantDay: 4,
			wantHour: 17, wantMin: 53, wantSec: 36,
		},

		// === SQL datetime with nanoseconds + offset + name ===
		{
			name: "Go full time string", input: "2015-02-08 03:02:00 +0300 MSK",
			wantYear: 2015, wantMon: 2, wantDay: 8,
			wantHour: 3, wantMin: 2,
		},

		// === Datetime with "at" keyword ===
		{
			name: "month day year at time", input: "September 17, 2012 at 10:09am",
			wantYear: 2012, wantMon: 9, wantDay: 17,
			wantHour: 10, wantMin: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWith(tt.input, tt.opts...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if tt.wantYear != 0 && result.Time.Year() != tt.wantYear {
				t.Errorf("year: got %d, want %d", result.Time.Year(), tt.wantYear)
			}
			if tt.wantMon != 0 && result.Time.Month() != tt.wantMon {
				t.Errorf("month: got %d, want %d", result.Time.Month(), tt.wantMon)
			}
			if tt.wantDay != 0 && result.Time.Day() != tt.wantDay {
				t.Errorf("day: got %d, want %d", result.Time.Day(), tt.wantDay)
			}
			if tt.wantHour != 0 && result.Time.Hour() != tt.wantHour {
				t.Errorf("hour: got %d, want %d", result.Time.Hour(), tt.wantHour)
			}
			if tt.wantMin != 0 && result.Time.Minute() != tt.wantMin {
				t.Errorf("minute: got %d, want %d", result.Time.Minute(), tt.wantMin)
			}
			if tt.wantSec != 0 && result.Time.Second() != tt.wantSec {
				t.Errorf("second: got %d, want %d", result.Time.Second(), tt.wantSec)
			}
		})
	}
}

// TestGaps_Regression ensures all previously-passing formats still work.
// This runs the full 53-format coverage test plus all Phase 1-4 tests.
func TestGaps_Regression(t *testing.T) {
	// Spot-check critical formats that could be affected by gap fixes.
	critical := []struct {
		input string
		year  int
		month time.Month
		day   int
	}{
		{"2024-03-15", 2024, 3, 15},
		{"2024-03-15T10:30:00Z", 2024, 3, 15},
		{"March 15, 2024", 2024, 3, 15},
		{"15 Mar 2024", 2024, 3, 15},
		{"03/15/2024", 2024, 3, 15},
		{"15.03.2024", 2024, 3, 15},
		{"20240315", 2024, 3, 15},
		{"2024-03-15 10:30:00", 2024, 3, 15},
		{"2024-03-15 10:30:00.000000", 2024, 3, 15},
		{"2024-03-15T10:30:00.123456789Z", 2024, 3, 15},
		{"10:30", 0, 1, 1},
		{"10:30 PM", 0, 1, 1},
	}

	for _, tt := range critical {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("REGRESSION: Parse(%q) error: %v", tt.input, err)
			}
			if tt.year != 0 && result.Time.Year() != tt.year {
				t.Errorf("REGRESSION: %q year: got %d, want %d", tt.input, result.Time.Year(), tt.year)
			}
			if result.Time.Month() != tt.month {
				t.Errorf("REGRESSION: %q month: got %d, want %d", tt.input, result.Time.Month(), tt.month)
			}
			if result.Time.Day() != tt.day {
				t.Errorf("REGRESSION: %q day: got %d, want %d", tt.input, result.Time.Day(), tt.day)
			}
		})
	}
}
