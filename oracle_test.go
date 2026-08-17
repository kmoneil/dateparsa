package dateparsa

import (
	"strings"
	"testing"
	"time"
)

// M8: compare Parse against an independent oracle.
//
// Every other correctness assertion in this package compares the library against
// itself, or against inputs the library generated. FuzzParse compares Parse to
// Layout.Parse and both run the same program, so a program that is wrong the same
// way twice passes. TestRoundTrip_Semantic renders its inputs with time.Format, so
// an input class no format renders does not exist in its corpus. The two
// FuzzCompile_* targets are real oracles and are pointed at Compile, where the
// token table bounds every field.
//
// C17 sat in the tree through all of that: "2024-03-15 10:30:00.99999999999999999999999"
// parsed to 2030-07-21 where time.Parse returns 2024-03-15, because nothing
// compared the two.
//
// The direction is one way only, and this is load bearing. CLAUDE.md: "Equivalence
// with time.Parse means agreement, not acceptance. ... Neither refusal fails them.
// time.Parse implements a superset of its own layout grammar that is written down
// nowhere ... Do not try to enumerate the stdlib's leniency; that enumeration grows
// one crasher at a time." So: where both accept, the instants must match. Where
// either refuses, there is nothing to compare.

// oracleLayout is the Go layout that describes a coverage case's input, for the
// cases whose Layout does not carry one. Keyed by input rather than by format
// name, because a format name does not determine a layout: DAY_MONTH_YEAR covers
// "15 March 2024", "15-Mar-2024", "Fri, 15 Mar 2024 10:30:00 +0000",
// "15/Mar/2024:10:30:00 +0000" and "15 Mar 24 10:30 UTC", which are five layouts
// under one name. C17's card prescribed a name-keyed table and that could not have
// worked.
var oracleLayout = map[string]string{
	"2024-03-15T10:30:00.123+05:30":   "2006-01-02T15:04:05.000-07:00",
	"Fri, 15 Mar 2024 10:30:00 +0000": time.RFC1123Z,
	"Friday, 15-Mar-24 10:30:00 UTC":  time.RFC850,
	"Fri Mar 15 10:30:00 2024":        time.ANSIC,
	"Fri Mar 15 10:30:00 UTC 2024":    time.UnixDate,
	"03/15/2024":                      "01/02/2006",
	"3/15/24":                         "1/2/06",
	"3/15/2024":                       "1/2/2006",
	"March 15, 2024":                  "January 2, 2006",
	"Mar 15, 2024":                    "Jan 2, 2006",
	"15/03/2024":                      "02/01/2006",
	"15.03.2024":                      "02.01.2006",
	"15 March 2024":                   "2 January 2006",
	"15-Mar-2024":                     "2-Jan-2006",
	"March 2024":                      "January 2006",
	"20240315T103000":                 "20060102T150405",
	"20240315103000":                  "20060102150405",
	"20240315T103000Z":                "20060102T150405Z",
	"Mar 15 10:30:00":                 "Jan 2 15:04:05",
	"15/Mar/2024:10:30:00 +0000":      "02/Jan/2006:15:04:05 -0700",
	"3/15/2024 10:30:00 AM":           "1/2/2006 03:04:05 PM",
	"15-Mar-2024 10:30":               "2-Jan-2006 15:04",
	"Fri Mar 15 10:30:00 EDT 2024":    time.UnixDate,
	"15 Mar 24 10:30 UTC":             time.RFC822,
	"Fri, 15 Mar 2024 10:30:00 UTC":   time.RFC1123,
	"Mar 15":                          "Jan 2",
	"15 Mar":                          "2 Jan",
}

// oracleExempt lists coverage inputs deliberately not compared, and why. An
// exemption is a decision written down; a format that is simply missing from both
// tables fails TestEveryCoverageCaseHasAnOracle.
var oracleExempt = map[string]string{
	"1710500000":     "a Unix timestamp is not a layout format; internal/epoch owns it",
	"1710500000000":  "same",
	"1710500000.123": "same",
	"2024-W11-5":     "Go's layout grammar has no ISO week token",
	"2024-W11":       "same",
	"2024-074":       "Go's layout grammar has no day-of-year token",
}

// TestEveryCoverageCaseHasAnOracle is the guard that stops the table above going
// stale. A format added to coverageCases, which the checklist for adding a format
// requires, is a format this test demands an oracle or an exemption for.
//
// It is the same shape as make fuzz-packages discovering its own targets: a
// hand-maintained list of what gets checked reads exactly like a complete one.
func TestEveryCoverageCaseHasAnOracle(t *testing.T) {
	for _, tc := range coverageCases {
		if _, exempt := oracleExempt[tc.input]; exempt {
			continue
		}
		if _, mapped := oracleLayout[tc.input]; mapped {
			continue
		}
		l, err := detectFor(tc)
		if err != nil {
			t.Errorf("%q: does not parse, so TestFormatCoverage should have failed first: %v",
				tc.input, err)
			continue
		}
		if _, ok := l.GoLayout(); !ok {
			t.Errorf("%q (%s) has no Go layout to compare against.\n"+
				"Add it to oracleLayout, or to oracleExempt with a reason.",
				tc.input, l)
		}
	}

	// And the other direction, so a stale entry cannot sit here forever claiming
	// to cover something.
	inputs := make(map[string]bool, len(coverageCases))
	for _, tc := range coverageCases {
		inputs[tc.input] = true
	}
	for in := range oracleLayout {
		if !inputs[in] {
			t.Errorf("oracleLayout has %q, which is not a coverage case any more", in)
		}
	}
	for in := range oracleExempt {
		if !inputs[in] {
			t.Errorf("oracleExempt has %q, which is not a coverage case any more", in)
		}
	}
}

// TestParseAgreesWithTimeParse runs the oracle over every advertised format.
func TestParseAgreesWithTimeParse(t *testing.T) {
	for _, tc := range coverageCases {
		if _, exempt := oracleExempt[tc.input]; exempt {
			continue
		}
		t.Run(tc.desc, func(t *testing.T) {
			r, err := parseFor(tc)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.input, err)
			}
			layout, ok := oracleLayout[tc.input]
			if !ok {
				layout, ok = r.Layout.GoLayout()
				if !ok {
					t.Fatalf("%q has no oracle; TestEveryCoverageCaseHasAnOracle "+
						"should have caught that", tc.input)
				}
			}
			assertAgrees(t, tc.input, layout, r.Time)
		})
	}
}

// TestParseAgreesWithTimeParseOnFractions is the assertion C17 needed.
//
// A fraction of one to nine digits is exact, and past nine both this library and
// the stdlib have to drop the rest. time.Parse truncates. parseFracSec used to
// accumulate every digit and then scale from the wrong place, so ten digits moved
// the answer by up to nine seconds and twenty wrapped int64 and moved it by years.
//
// All three shapes, because three separate detectors emit the field and only one of
// them bounded it.
func TestParseAgreesWithTimeParseOnFractions(t *testing.T) {
	shapes := []struct {
		name, prefix, suffix, layout string
	}{
		{
			name:   "GO_TIME_STRING",
			prefix: "2024-03-15 10:30:00.",
			suffix: " +0000 UTC",
			layout: "2006-01-02 15:04:05.999999999 -0700 MST",
		},
		{
			name:   "SQL_DATETIME",
			prefix: "2024-03-15 10:30:00.",
			suffix: "",
			layout: "2006-01-02 15:04:05.999999999",
		},
		{
			name:   "MONTH_DAY_YEAR",
			prefix: "March 15, 2024 10:30:00.",
			suffix: "",
			layout: "January 2, 2006 15:04:05.999999999",
		},
		{
			name:   "NUMERIC_MDY_TIME",
			prefix: "3/15/2024 10:30:00.",
			suffix: "",
			layout: "1/2/2006 15:04:05.999999999",
		},
		{
			name:   "ISO8601_FRAC",
			prefix: "2024-03-15T10:30:00.",
			suffix: "Z",
			layout: "2006-01-02T15:04:05.999999999Z",
		},
	}

	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			for n := 1; n <= 30; n++ {
				in := sh.prefix + strings.Repeat("9", n) + sh.suffix
				r, err := Parse(in)
				if err != nil {
					// Refusing an over-long fraction is allowed. Refusing a
					// fraction the stdlib reads exactly is not.
					if n <= 9 {
						if _, werr := time.Parse(sh.layout, in); werr == nil {
							t.Errorf("Parse(%q) refused a %d-digit fraction "+
								"time.Parse reads: %v", in, n, err)
						}
					}
					continue
				}
				assertAgrees(t, in, sh.layout, r.Time)
			}
		})
	}
}

// assertAgrees compares one instant against what time.Parse makes of the same
// input, and does nothing when the stdlib refuses.
//
// A layout with no year is compared with both years flattened. Parse fills the
// base year for a format that carries no year field, so that a Layout reproduces
// what the call which detected it returned; time.Parse leaves year 0. Neither is
// wrong and the difference is not what this test is about, so it is removed rather
// than exempted, which keeps the year-less formats covered for everything else.
func assertAgrees(t *testing.T, input, layout string, got time.Time) {
	t.Helper()

	want, err := time.Parse(layout, input)
	if err != nil {
		// The stdlib refused. Not a finding: this library is deliberately
		// stricter in places, and enumerating the stdlib's leniency is what
		// CLAUDE.md says not to do.
		return
	}

	// The stdlib is not a usable oracle for a timezone abbreviation it does not
	// recognise. time.Parse documents that it "records the time as being in a
	// fabricated location with the given zone abbreviation and a zero offset",
	// so "Fri Mar 15 10:30:00 EDT 2024" comes back as +0000 EDT, four hours from
	// what it says. This library resolves EDT from its own table of fixed
	// offsets, which SECURITY.md states as policy, and is the better answer.
	//
	// Found by this test on its first run, and it is the direction worth noting:
	// the oracle was wrong and the library was right. UTC, GMT and UCT really are
	// zero, so they are still compared.
	if name, offset := want.Zone(); offset == 0 && name != "" &&
		name != "UTC" && name != "GMT" && name != "UCT" {
		return
	}

	if want.Year() == 0 {
		got = flattenYear(got)
		want = flattenYear(want)
	}

	if !got.Equal(want) {
		t.Errorf("Parse and time.Parse disagree on %q\n"+
			"  layout      = %q\n"+
			"  Parse       = %v\n"+
			"  time.Parse  = %v\n"+
			"  difference  = %v",
			input, layout, got, want, got.Sub(want))
	}
}

func flattenYear(t time.Time) time.Time {
	return time.Date(0, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(),
		t.Nanosecond(), t.Location())
}

func detectFor(tc coverageCase) (*Layout, error) {
	if tc.dayFirst {
		return Detect(tc.input, WithPreferDayFirst(true))
	}
	return Detect(tc.input)
}

func parseFor(tc coverageCase) (ParseResult, error) {
	if tc.dayFirst {
		return ParseWith(tc.input, WithPreferDayFirst(true))
	}
	return Parse(tc.input)
}

// FuzzParseAgreesWithTimeParse is the oracle over arbitrary input.
//
// Its reach is narrower than the table tests above and that is worth stating
// rather than discovering: it can only compare a format whose Layout carries a Go
// layout, which is 18 of the 53 advertised formats. Every fallback detector passes
// an empty goLayout, so GO_TIME_STRING, the textual formats and the variable-width
// numeric ones are invisible here. They are covered by the tables, keyed by input,
// because a format name does not determine a layout.
//
// So this target is the part that generalises and the tables are the part that
// covers C17. Both, not either.
func FuzzParseAgreesWithTimeParse(f *testing.F) {
	seeds := []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00+05:30",
		"2024-03-15T10:30:00.123456789Z",
		"2024-03-15 10:30:00",
		"2024-03-15 10:30:00.000",
		"10:30:00",
		"20240315",

		// C17. Every one of these returned a wrong instant with a nil error.
		"2024-03-15 10:30:00.1234567890",
		"2024-03-15 10:30:00.99999999999999999999999",
		"2024-03-15 10:30:00.1234567890 +0000 UTC",
		"March 15, 2024 10:30:00.1234567890",
		"3/15/2024 10:30:00.1234567890",
		"15 Mar 2024 10:30:00.99999999999999999999999",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		r, err := Parse(input)
		if err != nil {
			return
		}
		if !reusable(r.Layout) {
			return
		}
		layout, ok := r.Layout.GoLayout()
		if !ok {
			return
		}
		assertAgrees(t, input, layout, r.Time)
	})
}
