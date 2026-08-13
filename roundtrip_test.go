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
	{name: "RFC3339", goFmt: time.RFC3339, checkFn: dateAndTime},
	{name: "RFC3339_NANO", goFmt: time.RFC3339Nano, checkFn: dateTimeAndNano},

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
	{name: "RFC2822", goFmt: "Mon, 02 Jan 2006 15:04:05 -0700", checkFn: dateAndTime},

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
			for i := 0; i < iterations; i++ {
				orig := randomTime(rng)

				// Render to string.
				var input string
				if spec.render != nil {
					input = spec.render(orig)
				} else {
					input = orig.Format(spec.goFmt)
				}

				// Parse back.
				result, err := ParseWith(input, spec.opts...)
				if err != nil {
					t.Errorf("iter %d: Parse(%q) error: %v", i, input, err)
					failures++
					if failures >= 5 {
						t.Fatalf("too many failures for %s, stopping", spec.name)
					}
					continue
				}

				// Compare.
				check := spec.checkFn
				parsed := result.Time.UTC()
				origUTC := orig.UTC()
				if err := check(origUTC, parsed); err != nil {
					t.Errorf("iter %d: input=%q\n  %v", i, input, err)
					failures++
					if failures >= 5 {
						t.Fatalf("too many failures for %s, stopping", spec.name)
					}
				}
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
