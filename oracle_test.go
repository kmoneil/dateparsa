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
		// The stdlib refused where this library accepted. That is the direction
		// this assertion used to return silently on, and C23 walked out through
		// the gap: "2024-02-30" was the first of March here and "day out of
		// range" there, on every date-bearing format, for as long as the library
		// has existed.
		//
		// CLAUDE.md's rule is about the other direction. "Do not try to
		// enumerate the stdlib's leniency" is advice about inputs *this* library
		// refuses, which is a decision it makes deliberately and often. An input
		// it *accepts* and the stdlib refuses is a claim about what the input
		// means, and there is nothing deliberate about it unless somebody wrote
		// down that there was.
		//
		// Before failing, canonicalise the input rather than loosening the
		// layout. Two things get canonicalised and they compose in this order.
		//
		// Padding, which F9 made a rule on every format: " 2024-03-15" is a
		// date here and "extra text" there. That is a difference about bytes
		// carrying no value, which is what this oracle is willing to look
		// past, and enumerating the inputs it can happen to would be every
		// format times every whitespace byte times both ends. The fuzzer found
		// "0000.01.01 " within one sweep of F9 landing, which is what a list
		// would have looked like on day one.
		//
		// Then the input's literal bytes to the ones the layout names, which is
		// what retryWithLayoutLiterals explains. It runs on the trimmed value
		// and not on input, because " 2024/03/15 " needs both and the padding
		// has to go first for the separator retry to see a layout-shaped input.
		value := trimPadding(input)
		trimmedWant, trimErr := time.Parse(layout, value)
		if trimErr == nil {
			want = trimmedWant
		} else if retried, ok := retryWithLayoutLiterals(value, layout); ok {
			want = retried
		} else {
			// The message quotes the stdlib's complaint about the layout it was
			// given, which for a class-matched literal is about the byte rather
			// than about the input. That is why the retry runs first: if it had
			// parsed, there would be no failure to report.
			if reason, exempt := oracleLenient[input]; exempt {
				t.Logf("%q: accepted here and refused by time.Parse, deliberately: %s",
					input, reason)
				return
			}
			t.Errorf("Parse accepted %q and time.Parse refused it\n"+
				"  layout     = %q\n"+
				"  Parse      = %v\n"+
				"  time.Parse = %v\n"+
				"  This library is documented as the stricter of the two. If this\n"+
				"  input is one it should accept, add it to oracleLenient with the\n"+
				"  reason; otherwise it is a bug in what this library accepts.",
				input, layout, got, err)
			return
		}
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

// oracleLenient lists inputs this library accepts on purpose where time.Parse
// refuses, with the reason. Anything not on it that diverges is a finding.
//
// **It is empty, and that is the result rather than an oversight.** The list was
// built by running the assertion over every coverage case, over 200 generated
// samples of each of the 31 round-trip formats, and over the committed fuzz
// corpus, and collecting what came out. What came out was three inputs, all of
// them one defect in Layout.GoLayout rather than lenience, and the separator
// retry above turns those into comparisons instead of exemptions.
//
// F9 is the reason it is still empty rather than eight entries long, and the
// distinction is worth keeping. Surrounding whitespace *is* a deliberate
// divergence: this library reads " 2024-03-15" as a date and time.Parse calls
// it extra text. But it is a rule and not a list, so it belongs in the
// canonicalisation above beside the separator class and the case of AM/PM, and
// putting it here instead was the first version of that change. The fuzzer
// refuted it inside one sweep with "0000.01.01 ", an input no list written by
// hand would have held, and every format times every whitespace byte times both
// ends is what the list would have had to grow to.
//
// So this list is for a divergence that is genuinely about one input. A rule
// goes above; anything that needs an entry here needs a sentence saying why
// that input means what this library says it means, and CLAUDE.md's equivalence
// invariant should gain it too.
var oracleLenient = map[string]string{}

// TestPaddedInputAgreesWithTheTrimmedValue is F9 at the oracle.
//
// The claim being checked is narrow and is the whole of what F9 asks for: this
// library accepts padding the stdlib refuses, and answers with the instant the
// stdlib gives for the value inside it. More inputs, not more meanings.
//
// It is a test rather than a list because C27 is two cards ago: a documented
// format changed what it reports with 31 round-trip formats, 23 fuzz targets
// and the oracle all green, because the gates were pointed somewhere else. The
// premise check is the other half of that. If a Go release starts accepting
// padded input, this says so instead of the canonicalisation quietly covering
// nothing.
func TestPaddedInputAgreesWithTheTrimmedValue(t *testing.T) {
	cases := []struct{ input, layout string }{
		{" 2024-03-15", "2006-01-02"},
		{"2024-03-15 ", "2006-01-02"},
		{"  2024-03-15  ", "2006-01-02"},
		{"2024-03-15\r\n", "2006-01-02"},
		{"\t2024-03-15T10:30:00Z", "2006-01-02T15:04:05Z"},
		{"2024-03-15T10:30:00Z ", "2006-01-02T15:04:05Z"},
		{" 03/15/2024 ", "01/02/2006"},
		{"\n15 Mar 2024\n", "2 Jan 2006"},

		// Padding and a separator class at once, which is why the padding is
		// canonicalised before the literal retry rather than after it.
		{" 2024/03/15 ", "2006-01-02"},
		{"\t2024.03.15\n", "2006-01-02"},

		// The crasher. Found by FuzzParseAgreesWithTimeParse on the first sweep
		// after F9 landed, against the version that kept a hand-written list.
		{"0000.01.01 ", "2006-01-02"},
	}

	for _, c := range cases {
		r, err := Parse(c.input)
		if err != nil {
			t.Errorf("Parse(%q): %v, want the trimmed value's answer", c.input, err)
			continue
		}

		// The premise: the stdlib really does refuse this, so the
		// canonicalisation is doing something.
		if _, stdErr := time.Parse(c.layout, c.input); stdErr == nil {
			t.Errorf("time.Parse(%q, %q) now succeeds; this input no longer shows the "+
				"divergence it was chosen for", c.layout, c.input)
		}

		// And through the oracle, which is what would fail if the
		// canonicalisation were removed without the behaviour changing.
		assertAgrees(t, c.input, c.layout, r.Time)
	}
}

// retryWithLayoutLiterals re-runs the stdlib with the input's literal bytes
// replaced by the ones the layout names, and reports whether that parses.
//
// A trie entry matches a literal by class rather than by byte. That is
// deliberate and is why "2024/03/15" and "2024.03.15" parse at all: naming "-"
// in the entry would refuse them. The stdlib has no such class, so it refuses
// them for a reason that is about which byte was written and not about what the
// input means, and the instant is what this oracle is for.
//
// So the input is canonicalised rather than the layout being loosened. Writing
// the input's byte into the layout was the first version and it is the wrong
// direction: "-07" is a zone token, so rewriting its "-" to the input's "+"
// turns it into the literal "+07" and the layout stops describing anything.
// Rewriting the input cannot damage the layout.
//
// Only non-alphanumeric bytes are substituted, and "T", which is the one letter
// a Go layout uses as a literal. Digits are never touched, so a token's value
// cannot be rewritten: an earlier version substituted wherever the rendered
// byte matched the layout byte, "05" renders "00" for second zero, the leading
// digit coincided, and the input's "60" went into the token. The stdlib then
// answered a minute earlier than the disagreement it was being asked about.
//
// **How wide the class should be is a separate question and is not settled
// here.** It is what lets "0000.01.01+00:00:00" and "00:00:00/000" parse, which
// no producer writes. This function keeps the oracle pointed at instants; the
// acceptance question is filed on its own.
func retryWithLayoutLiterals(input, layout string) (time.Time, bool) {
	if len(input) != len(layout) {
		return time.Time{}, false
	}
	literal := func(c byte) bool {
		switch {
		case c >= '0' && c <= '9':
			return false
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			return c == 'T'
		}
		return true
	}
	// The zone token's first byte is part of its value rather than a separator:
	// "-07" reads "+01" as one hour east, so canonicalising the input's "+" to
	// the layout's "-" flips the offset and reports a two-hour disagreement that
	// is this function's own doing. Found by the fuzzer on
	// "0000.01.01 00:00:00+01". The fraction's "." is a separator and not a
	// value, so it is left rewritable, which is what turns "00:00:00/000" into a
	// comparison.
	zoneToken := func(l string, i int) bool {
		for _, tok := range [...]string{"-07:00:00", "-070000", "-07:00", "-0700", "-07"} {
			if strings.HasPrefix(l[i:], tok) {
				return true
			}
		}
		return false
	}
	b := []byte(input)
	changed := false
	// AM/PM is the one token the stdlib matches case-sensitively: "PM" takes
	// "AM" and "PM" and nothing else, "pm" takes the lower-case pair. Every
	// other letter token it matches with a case-insensitive compare, month and
	// day names included, so this is a quirk of one token rather than a
	// difference of opinion about the input. This library folds case
	// everywhere, deliberately, because "01:00 Am" is a thing that gets
	// written.
	for i := 0; i+1 < len(layout); i++ {
		if layout[i] != 'P' && layout[i] != 'p' || layout[i+1] != 'M' && layout[i+1] != 'm' {
			continue
		}
		up := layout[i] == 'P'
		for j := i; j < i+2; j++ {
			c := b[j]
			if up && c >= 'a' && c <= 'z' {
				c -= 32
			} else if !up && c >= 'A' && c <= 'Z' {
				c += 32
			}
			if c != b[j] {
				b[j] = c
				changed = true
			}
		}
	}
	for i := range b {
		if zoneToken(layout, i) {
			continue
		}
		if literal(b[i]) && literal(layout[i]) && b[i] != layout[i] {
			b[i] = layout[i]
			changed = true
		}
	}
	if !changed {
		return time.Time{}, false
	}
	want, err := time.Parse(layout, string(b))
	return want, err == nil
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

		// C23. Every one of these was the first of the next month, with a nil
		// error, where time.Parse says "day out of range". They are seeds rather
		// than only a table because this target is what would have found them:
		// the table tests parse inputs somebody chose, and nobody chooses a date
		// that does not exist.
		"2024-02-30",
		"2024-02-31",
		"2023-02-29",
		"1900-02-29",
		"2024-04-31",
		"2024-02-30T10:30:00Z",
		"2024-02-30 10:30:00",
		"20240230",

		// The leap second, which the over-acceptance assertion found on its
		// first two-minute run. Accepting 60 was deliberate; answering
		// 2017-01-01T00:00:00Z for it was not.
		"2016-12-31T23:59:60Z",
		"2024-06-30 23:59:60",
		"20240630235960",

		// The separator classes, which is what the retry inside assertAgrees is
		// for: one trie entry accepts all three and reports the dashed layout.
		"2024/03/15",
		"2024.03.15",
		"2024/03/15 10:30:00",

		// F9, found by this target on the first sweep after it landed and
		// against the version of it that kept a hand-written exemption list.
		// Padding is canonicalised in assertAgrees now, for the same reason the
		// separators above are, and TestPaddedInputAgreesWithTheTrimmedValue
		// carries this input as a fixed case as well.
		"0000.01.01 ",
		" 2024-03-15",
		"\t2024/03/15 10:30:00\n",
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
