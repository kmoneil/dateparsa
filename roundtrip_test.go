package dateparsa

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// formatSpec defines a date format for round-trip testing.
type formatSpec struct {
	name    string
	goFmt   string                             // Go format string for rendering
	render  func(t time.Time) string           // Custom renderer (overrides goFmt)
	opts    []Option                           // Parse options needed
	checkFn func(orig, parsed time.Time) error // Custom comparison (default: full match)

	// cases are run before the random ones, for boundaries the generator
	// cannot reach. randomTime picks a day between 1 and 28 of a month between
	// 1 and 12, which never renders day-of-year 365 or 366, never renders ISO
	// week 53, and never renders a week 1 that begins in the previous calendar
	// year. Those are exactly where the arithmetic these formats need is
	// wrong, so a spec without them checks the easy half.
	cases []time.Time

	// zoned says the format writes a numeric zone offset, so the generator
	// renders each sample in a random zone instead of in UTC.
	//
	// It exists because randomTime returns a UTC time and every zone-bearing
	// format therefore rendered "Z" or "+0000" on all 1000 iterations. The
	// suite CLAUDE.md calls the one that catches a silent wrong answer had
	// never round-tripped an offset that was not zero, on any format, and a
	// wrong offset is a wrong instant.
	zoned bool
}

// dateOnly compares year/month/day only.
func dateOnly(orig, parsed time.Time) error {
	if parsed.Year() != orig.Year() || parsed.Month() != orig.Month() || parsed.Day() != orig.Day() {
		return fmt.Errorf("date mismatch: got %d-%02d-%02d, want %d-%02d-%02d",
			parsed.Year(), parsed.Month(), parsed.Day(),
			orig.Year(), orig.Month(), orig.Day())
	}
	return nil
}

// dateAndTime compares year/month/day/hour/minute/second.
func dateAndTime(orig, parsed time.Time) error {
	if err := dateOnly(orig, parsed); err != nil {
		return err
	}
	if parsed.Hour() != orig.Hour() || parsed.Minute() != orig.Minute() || parsed.Second() != orig.Second() {
		return fmt.Errorf("time mismatch: got %02d:%02d:%02d, want %02d:%02d:%02d",
			parsed.Hour(), parsed.Minute(), parsed.Second(),
			orig.Hour(), orig.Minute(), orig.Second())
	}
	return nil
}

// dateTimeAndNano compares everything including nanoseconds.
func dateTimeAndNano(orig, parsed time.Time) error {
	if err := dateAndTime(orig, parsed); err != nil {
		return err
	}
	// Compare milliseconds (fractional formats vary in precision).
	origMs := orig.Nanosecond() / 1e6
	parsedMs := parsed.Nanosecond() / 1e6
	if origMs != parsedMs {
		return fmt.Errorf("millis mismatch: got %d, want %d", parsedMs, origMs)
	}
	return nil
}

// timeOnly compares hour/minute/second only (ignores date).
func timeOnly(orig, parsed time.Time) error {
	if parsed.Hour() != orig.Hour() || parsed.Minute() != orig.Minute() || parsed.Second() != orig.Second() {
		return fmt.Errorf("time mismatch: got %02d:%02d:%02d, want %02d:%02d:%02d",
			parsed.Hour(), parsed.Minute(), parsed.Second(),
			orig.Hour(), orig.Minute(), orig.Second())
	}
	return nil
}

var roundTripFormats = []formatSpec{
	// === ISO 8601 / RFC 3339 ===
	{name: "ISO_DATE", goFmt: "2006-01-02", checkFn: dateOnly},
	{name: "ISO_DATETIME", goFmt: "2006-01-02T15:04:05", checkFn: dateAndTime},
	{name: "ISO_DATETIME_Z", goFmt: "2006-01-02T15:04:05Z", checkFn: dateAndTime},
	{name: "RFC3339", goFmt: time.RFC3339, checkFn: dateAndTime, zoned: true},
	{name: "RFC3339_NANO", goFmt: time.RFC3339Nano, checkFn: dateTimeAndNano, zoned: true},

	// === SQL ===
	{name: "SQL_DATETIME", goFmt: "2006-01-02 15:04:05", checkFn: dateAndTime},
	{name: "SQL_DATETIME_FRAC3", goFmt: "2006-01-02 15:04:05.000", checkFn: dateTimeAndNano},

	// === Compact ===
	{name: "COMPACT_DATE", goFmt: "20060102", checkFn: dateOnly},
	{name: "COMPACT_DATETIME", goFmt: "20060102T150405", checkFn: dateAndTime},
	{
		name:    "COMPACT_DATETIME_Z",
		render:  func(t time.Time) string { return t.UTC().Format("20060102T150405") + "Z" },
		checkFn: dateAndTime,
	},

	// === US numeric (fixed-width) ===
	{name: "US_SLASH", goFmt: "01/02/2006", checkFn: dateOnly},

	// === European ===
	{
		name: "EUROPEAN_DOT", goFmt: "02.01.2006",
		opts: []Option{WithPreferDayFirst(true)}, checkFn: dateOnly,
	},

	// === Textual month ===
	{name: "MONTH_DAY_YEAR", goFmt: "January 2, 2006", checkFn: dateOnly},
	{name: "ABBR_MONTH_DAY_YEAR", goFmt: "Jan 2, 2006", checkFn: dateOnly},
	{name: "DAY_MONTH_YEAR", goFmt: "2 January 2006", checkFn: dateOnly},

	// === RFC 2822 ===
	{name: "RFC2822", goFmt: "Mon, 02 Jan 2006 15:04:05 -0700", checkFn: dateAndTime, zoned: true},

	// === RFC 1123 ===
	{
		name:    "RFC1123_UTC",
		render:  func(t time.Time) string { return t.UTC().Format("Mon, 02 Jan 2006 15:04:05") + " UTC" },
		checkFn: dateAndTime,
	},

	// === ANSIC ===
	{
		name:    "ANSIC",
		render:  func(t time.Time) string { return t.UTC().Format("Mon Jan _2 15:04:05 2006") },
		checkFn: dateAndTime,
	},

	// === Time only ===
	{name: "TIME_HM", goFmt: "15:04", checkFn: func(orig, parsed time.Time) error {
		if parsed.Hour() != orig.Hour() || parsed.Minute() != orig.Minute() {
			return fmt.Errorf("HH:MM mismatch: got %02d:%02d, want %02d:%02d",
				parsed.Hour(), parsed.Minute(), orig.Hour(), orig.Minute())
		}
		return nil
	}},
	{name: "TIME_HMS", goFmt: "15:04:05", checkFn: timeOnly},
	{
		name: "TIME_12H",
		render: func(t time.Time) string {
			h := t.Hour()
			suffix := "AM"
			if h >= 12 {
				suffix = "PM"
				if h > 12 {
					h -= 12
				}
			}
			if h == 0 {
				h = 12
			}
			return fmt.Sprintf("%02d:%02d %s", h, t.Minute(), suffix)
		},
		checkFn: func(orig, parsed time.Time) error {
			if parsed.Hour() != orig.Hour() || parsed.Minute() != orig.Minute() {
				return fmt.Errorf("12h mismatch: got %02d:%02d, want %02d:%02d",
					parsed.Hour(), parsed.Minute(), orig.Hour(), orig.Minute())
			}
			return nil
		},
	},

	// === EXIF ===
	{
		name:    "EXIF_DATE",
		render:  func(t time.Time) string { return t.Format("2006:01:02") },
		checkFn: dateOnly,
	},
	{
		name:    "EXIF_DATETIME",
		render:  func(t time.Time) string { return t.Format("2006:01:02 15:04:05") },
		checkFn: dateAndTime,
	},

	// === Comma fractional (Java/log4j) ===
	{
		name: "COMMA_FRAC",
		render: func(t time.Time) string {
			ms := t.Nanosecond() / 1e6
			return t.Format("2006-01-02 15:04:05") + fmt.Sprintf(",%03d", ms)
		},
		checkFn: dateTimeAndNano,
	},

	// === ISO partial ===
	{
		name: "YEAR_MONTH", goFmt: "2006-01",
		checkFn: func(orig, parsed time.Time) error {
			if parsed.Year() != orig.Year() || parsed.Month() != orig.Month() {
				return fmt.Errorf("year-month mismatch: got %d-%02d, want %d-%02d",
					parsed.Year(), parsed.Month(), orig.Year(), orig.Month())
			}
			return nil
		},
	},

	// === SQL datetime + AM/PM ===
	{
		name: "SQL_AMPM",
		render: func(t time.Time) string {
			h := t.Hour()
			suffix := "AM"
			if h >= 12 {
				suffix = "PM"
				if h > 12 {
					h -= 12
				}
			}
			if h == 0 {
				h = 12
			}
			return fmt.Sprintf("%s %02d:%02d:%02d %s", t.Format("2006-01-02"), h, t.Minute(), t.Second(), suffix)
		},
		checkFn: dateAndTime,
	},

	// === SQL datetime + TZ name ===
	{
		name:    "SQL_TZ_NAME",
		render:  func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") + " UTC" },
		checkFn: dateAndTime,
	},

	// === Common Log Format ===
	{
		name:    "CLF",
		render:  func(t time.Time) string { return t.UTC().Format("02/Jan/2006:15:04:05") + " +0000" },
		checkFn: dateAndTime,
	},

	// === ISO week and ordinal ===
	//
	// These three had no spec at all until 2026-08-14, which is why the range
	// checks behind them were never exercised against anything but a value
	// they happened to accept. Detect tries them first, ahead of every other
	// format, so nothing about them was obscure. Nothing checked them.
	{
		name:    "ISO_ORDINAL",
		render:  func(t time.Time) string { return t.UTC().Format("2006-002") },
		checkFn: dateOnly,
		cases: []time.Time{
			day(2023, 1, 1),   // 001
			day(2024, 2, 29),  // 060, the leap day itself
			day(2023, 12, 31), // 365, the last day of a common year
			day(2024, 12, 30), // 365 of a leap year, which is not its last day
			day(2024, 12, 31), // 366, the only day-of-year that depends on the year
			day(1900, 12, 31), // 365: 1900 is divisible by 100 and not by 400
			day(2000, 12, 31), // 366: 2000 is divisible by 400
		},
	},
	{
		name:    "ISO_WEEK",
		render:  renderISOWeek,
		checkFn: weekStart,
		cases:   isoWeekBoundaryCases,
	},
	{
		name:    "ISO_WEEK_DATE",
		render:  renderISOWeekDate,
		checkFn: dateOnly,
		cases:   isoWeekBoundaryCases,
	},
}

// day is a UTC midnight, for the explicit case lists.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// isoWeekBoundaryCases are the dates where the ISO week-numbering year and the
// calendar year disagree, which is the arithmetic isoWeekToDate has to get
// right and the generator never produces.
var isoWeekBoundaryCases = []time.Time{
	day(2021, 1, 4),   // 2021-W01-1, week 1 beginning in its own calendar year
	day(2015, 1, 1),   // 2015-W01-4, a Thursday, so 2015 has 53 weeks
	day(2014, 12, 29), // 2015-W01-1: ISO year 2015, calendar year 2014
	day(2019, 12, 30), // 2020-W01-1, the same one year later
	day(2020, 12, 28), // 2020-W53-1: 2020 began on a Wednesday and is a leap year
	day(2021, 1, 3),   // 2020-W53-7: ISO year 2020, calendar year 2021
	day(2024, 12, 29), // 2024-W52-7, the last day of the last week of a 52-week year
	day(2024, 12, 30), // 2025-W01-1
}

// isoWeekday is the ISO numbering, Monday 1 through Sunday 7. Go's Weekday
// puts Sunday at 0.
func isoWeekday(t time.Time) int {
	if wd := int(t.Weekday()); wd != 0 {
		return wd
	}
	return 7
}

// mondayOf returns the Monday of t's ISO week, which is the instant a
// week-without-a-day names.
func mondayOf(t time.Time) time.Time {
	return t.AddDate(0, 0, -(isoWeekday(t) - 1))
}

// renderISOWeek writes the ISO week-numbering year, which is not always the
// calendar year: 2014-12-29 is 2015-W01.
func renderISOWeek(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

func renderISOWeekDate(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d-%d", y, w, isoWeekday(t.UTC()))
}

// weekStart compares against the Monday of the original's ISO week, because a
// week number with no weekday names that Monday and nothing finer.
func weekStart(orig, parsed time.Time) error {
	return dateOnly(mondayOf(orig), parsed)
}

// randomZone returns a fixed zone at a random whole-minute offset in the range
// parseTZOffset accepts, which is -23:59 to +23:59 because RFC 3339 allows it.
//
// It draws from the whole range on purpose rather than from the offsets in use
// today. The executor holds 105 pre-built Locations at 15-minute granularity and
// builds anything else on demand, so an offset off that grid takes a different
// path through the code, and a generator that only produced real-world offsets
// would take the pre-built one about nine times in ten. +05:53 is Bombay before
// 1955 and -00:44 Monrovia before 1972, so off-grid is historical data rather
// than a fiction, and M9 is the card about a gate that never reached it.
func randomZone(rng *rand.Rand) *time.Location {
	minutes := rng.Intn(2*1439+1) - 1439
	return time.FixedZone("GEN", minutes*60)
}

// renderSample renders orig with spec's format, in a random zone where the
// format writes one.
func renderSample(spec formatSpec, orig time.Time, rng *rand.Rand) string {
	if spec.zoned {
		orig = orig.In(randomZone(rng))
	}
	if spec.render != nil {
		return spec.render(orig)
	}
	return orig.Format(spec.goFmt)
}

// randomTime generates a random time between 1970 and 2099.
func randomTime(rng *rand.Rand) time.Time {
	year := 1970 + rng.Intn(130) // 1970-2099
	month := 1 + rng.Intn(12)    // 1-12
	day := 1 + rng.Intn(28)      // 1-28 (safe for all months)
	hour := rng.Intn(24)         // 0-23
	minute := rng.Intn(60)       // 0-59
	second := rng.Intn(60)       // 0-59
	ms := rng.Intn(1000)         // 0-999 milliseconds
	return time.Date(year, time.Month(month), day, hour, minute, second, ms*1e6, time.UTC)
}

// TestRoundTrip_Semantic runs the semantic round-trip fuzzer.
// For each format, generates N random dates, formats them, parses them back,
// and verifies the result matches.
func TestRoundTrip_Semantic(t *testing.T) {
	const iterations = 1000
	rng := rand.New(rand.NewSource(42)) // deterministic seed for reproducibility

	for _, spec := range roundTripFormats {
		t.Run(spec.name, func(t *testing.T) {
			failures := 0

			// One round trip: render orig, parse it back, compare. label
			// distinguishes an explicit boundary case from a generated one,
			// because a failing boundary is a different report from a failing
			// iteration and the two want different follow-up.
			trip := func(label string, orig time.Time) {
				input := renderSample(spec, orig, rng)

				result, err := ParseWith(input, spec.opts...)
				if err != nil {
					t.Errorf("%s: Parse(%q) error: %v", label, input, err)
					failures++
					if failures >= 5 {
						t.Fatalf("too many failures for %s, stopping", spec.name)
					}
					return
				}

				if err := spec.checkFn(orig.UTC(), result.Time.UTC()); err != nil {
					t.Errorf("%s: input=%q\n  %v", label, input, err)
					failures++
					if failures >= 5 {
						t.Fatalf("too many failures for %s, stopping", spec.name)
					}
				}
			}

			for _, c := range spec.cases {
				trip("case "+c.UTC().Format("2006-01-02"), c)
			}
			for i := 0; i < iterations; i++ {
				trip(fmt.Sprintf("iter %d", i), randomTime(rng))
			}
		})
	}
}

// TestRoundTrip_LayoutReuse verifies that a Layout detected from one date
// correctly parses a different date in the same format.
func TestRoundTrip_LayoutReuse(t *testing.T) {
	const iterations = 500
	rng := rand.New(rand.NewSource(99))

	formats := []struct {
		name    string
		goFmt   string
		checkFn func(orig, parsed time.Time) error
	}{
		{"ISO_DATE", "2006-01-02", dateOnly},
		{"ISO_DATETIME_Z", "2006-01-02T15:04:05Z", dateAndTime},
		{"SQL_DATETIME", "2006-01-02 15:04:05", dateAndTime},
		{"COMPACT_DATE", "20060102", dateOnly},
	}

	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			// Detect layout from a seed date.
			seed := randomTime(rng)
			seedStr := seed.UTC().Format(f.goFmt)
			result, err := Parse(seedStr)
			if err != nil {
				t.Fatalf("seed Parse(%q) error: %v", seedStr, err)
			}
			layout := result.Layout
			if layout == nil {
				t.Fatalf("seed Parse(%q) returned nil Layout", seedStr)
			}

			// Parse N random dates with the same layout.
			failures := 0
			for i := 0; i < iterations; i++ {
				orig := randomTime(rng).UTC()
				input := orig.Format(f.goFmt)

				parsed, err := layout.Parse(input)
				if err != nil {
					t.Errorf("iter %d: Layout.Parse(%q) error: %v", i, input, err)
					failures++
					if failures >= 5 {
						t.Fatal("too many failures")
					}
					continue
				}

				if err := f.checkFn(orig, parsed.UTC()); err != nil {
					t.Errorf("iter %d: input=%q\n  %v", i, input, err)
					failures++
					if failures >= 5 {
						t.Fatal("too many failures")
					}
				}
			}
		})
	}
}

// FuzzRoundTrip_ISO is a Go fuzz target that generates random dates,
// formats them as ISO 8601, and verifies the round-trip.
func FuzzRoundTrip_ISO(f *testing.F) {
	f.Add(int64(0), int64(0))
	f.Add(int64(1710504800), int64(123000000))
	f.Add(int64(946684800), int64(0))    // 2000-01-01
	f.Add(int64(-62135596800), int64(0)) // year 1

	f.Fuzz(func(t *testing.T, sec, nsec int64) {
		// Constrain to reasonable range.
		if sec < -62135596800 || sec > 4102444800 { // year 1 to 2100
			return
		}
		if nsec < 0 || nsec >= 1e9 {
			return
		}

		orig := time.Unix(sec, nsec).UTC()

		// ISO date round-trip.
		input := orig.Format("2006-01-02")
		result, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", input, err)
		}
		parsed := result.Time.UTC()
		if parsed.Year() != orig.Year() || parsed.Month() != orig.Month() || parsed.Day() != orig.Day() {
			t.Fatalf("ISO date round-trip failed: input=%q got=%v want=%v", input, parsed, orig)
		}

		// ISO datetime round-trip.
		input2 := orig.Format(time.RFC3339)
		result2, err := Parse(input2)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", input2, err)
		}
		parsed2 := result2.Time.UTC()
		if parsed2.Year() != orig.Year() || parsed2.Month() != orig.Month() || parsed2.Day() != orig.Day() ||
			parsed2.Hour() != orig.Hour() || parsed2.Minute() != orig.Minute() || parsed2.Second() != orig.Second() {
			t.Fatalf("RFC3339 round-trip failed: input=%q got=%v want=%v", input2, parsed2, orig)
		}
	})
}

// FuzzRoundTrip_SQL is a Go fuzz target for SQL datetime format.
func FuzzRoundTrip_SQL(f *testing.F) {
	f.Add(int64(1710504800))
	f.Add(int64(0))
	f.Add(int64(946684800))

	f.Fuzz(func(t *testing.T, sec int64) {
		if sec < 0 || sec > 4102444800 {
			return
		}
		orig := time.Unix(sec, 0).UTC()

		input := orig.Format("2006-01-02 15:04:05")
		result, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", input, err)
		}
		parsed := result.Time.UTC()
		if !parsed.Equal(orig) {
			t.Fatalf("SQL round-trip failed: input=%q got=%v want=%v", input, parsed, orig)
		}
	})
}
