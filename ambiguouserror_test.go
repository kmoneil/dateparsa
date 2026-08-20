package dateparsa

import (
	"errors"
	"testing"
	"time"
)

// TestStrictModeCarriesTheCallerConfig is C21 half two.
//
// buildAmbiguousError rebuilt detect.Config from two of cfg's fields and dropped
// the rest. Locales was the one that mattered: without them neither re-detection
// can find a locale month name, so both failed, the interpretation list came back
// empty, and the function fell through to a *ParseError reading "ambiguous date
// could not be interpreted". Strict mode did not refuse to guess about "mai 15",
// it failed to parse it, while the lenient path read it and reported the guess.
//
// A caller who turned strict mode on because SECURITY.md told them to lost the
// ability to parse non-English textual dates at all.
func TestStrictModeCarriesTheCallerConfig(t *testing.T) {
	cases := []struct {
		in      string
		locales []Locale
	}{
		{"mai 15", []Locale{FR}},
		{"15 mai", []Locale{FR}},
		{"marzo 15", []Locale{ES}},
		{"mai 15", []Locale{FR, ES}},
	}

	for _, c := range cases {
		// The lenient parse is the premise: if this stops being ambiguous, the
		// test below is passing for the wrong reason.
		lenient, err := ParseWith(c.in, WithLocales(c.locales...))
		if err != nil {
			t.Fatalf("test premise gone: ParseWith(%q) does not parse: %v", c.in, err)
		}
		if !lenient.Ambiguous {
			t.Fatalf("test premise gone: ParseWith(%q) is no longer ambiguous", c.in)
		}

		_, err = ParseWith(c.in, WithLocales(c.locales...), WithStrictMode(true))
		if err == nil {
			t.Errorf("ParseWith(%q, strict) accepted an ambiguous value", c.in)
			continue
		}

		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v\n"+
				"  want *AmbiguousDateError; a strict-mode refusal has to carry the\n"+
				"  readings it refused to choose between, not report that it could\n"+
				"  not interpret an input the lenient path reads",
				c.in, err, err)
			continue
		}
		if len(ade.Interpretations) == 0 {
			t.Errorf("ParseWith(%q, strict): no interpretations", c.in)
		}
		if !errors.Is(err, ErrAmbiguous) {
			t.Errorf("ParseWith(%q, strict): does not unwrap to ErrAmbiguous", c.in)
		}
	}
}

// TestStrictModeNumericStillCarriesBothReadings is the case that already worked, so
// that carrying the config cannot have broken it.
func TestStrictModeNumericStillCarriesBothReadings(t *testing.T) {
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	var ade *AmbiguousDateError
	if !errors.As(err, &ade) {
		t.Fatalf("want *AmbiguousDateError, got %T %v", err, err)
	}
	if len(ade.Interpretations) != 2 {
		t.Fatalf("got %d interpretations, want 2", len(ade.Interpretations))
	}
	if ade.Interpretations[0].Time.Equal(ade.Interpretations[1].Time) {
		t.Errorf("both interpretations are %v; the numeric case is the one that "+
			"has always offered two different readings",
			ade.Interpretations[0].Time)
	}
	byLabel := map[string]string{}
	for _, i := range ade.Interpretations {
		byLabel[i.Label] = i.Time.Format("2006-01-02")
	}
	if byLabel["MM/DD/YYYY"] != "2024-01-02" || byLabel["DD/MM/YYYY"] != "2024-02-01" {
		t.Errorf("interpretations = %v, want MM/DD/YYYY 2024-01-02 and DD/MM/YYYY 2024-02-01",
			byLabel)
	}
}

// TestStrictModeTextualCarriesTheYearReading is C21 half one.
//
// "MAY 15" is ambiguous because 15 could be the fifteenth of May or the year 2015,
// which is what textualDayIsAGuess decides. The error used to offer two
// interpretations that were the same instant, under labels naming a numeric format
// the input is not, and the reading it was really choosing between was not among
// them. A caller checking whether the two readings agree concluded the guess was
// safe and took it.
//
// This replaces TestStrictModeTextualInterpretationsAreStillWrong, which pinned that
// behaviour and was written to fail when it was fixed.
func TestStrictModeTextualCarriesTheYearReading(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in        string
		locales   []Locale
		dayLabel  string
		dayTime   string
		yearLabel string
		yearTime  string
	}{
		{"MAY 15", nil, "MONTH_DAY", "2026-05-15", "MONTH_YEAR", "2015-05-01"},
		{"March 15", nil, "MONTH_DAY", "2026-03-15", "MONTH_YEAR", "2015-03-01"},
		{"December 25", nil, "MONTH_DAY", "2026-12-25", "MONTH_YEAR", "2025-12-01"},
		{"Sept 09", nil, "MONTH_DAY", "2026-09-09", "MONTH_YEAR", "2009-09-01"},
		{"MAY15", nil, "MONTH_DAY", "2026-05-15", "MONTH_YEAR", "2015-05-01"},
		{"15 March", nil, "DAY_MONTH", "2026-03-15", "YEAR_MONTH", "2015-03-01"},
		{"mai 15", []Locale{FR}, "MONTH_DAY", "2026-05-15", "MONTH_YEAR", "2015-05-01"},
		{"15 mai", []Locale{FR}, "DAY_MONTH", "2026-05-15", "YEAR_MONTH", "2015-05-01"},
		{"marzo 15", []Locale{ES}, "MONTH_DAY", "2026-03-15", "MONTH_YEAR", "2015-03-01"},
	}

	for _, c := range cases {
		opts := []Option{WithBaseTime(base), WithLocales(c.locales...), WithStrictMode(true)}
		_, err := ParseWith(c.in, opts...)
		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v, want *AmbiguousDateError", c.in, err, err)
			continue
		}
		got := map[string]string{}
		for _, i := range ade.Interpretations {
			got[i.Label] = i.Time.Format("2006-01-02")
		}
		if got[c.dayLabel] != c.dayTime || got[c.yearLabel] != c.yearTime || len(got) != 2 {
			t.Errorf("ParseWith(%q, strict) interpretations = %v\n"+
				"  want %s %s and %s %s: the number is either the day or a two-digit year,\n"+
				"  and both readings have to be in the error for the caller to choose",
				c.in, got, c.dayLabel, c.dayTime, c.yearLabel, c.yearTime)
		}
	}
}

// TestStrictModeTwoNumbersCarriesBothReadings is C27.
//
// "MAY10" is one number beside a month name and has always reported the
// day-or-year question. "01MAY10" is the same question asked of two numbers,
// where the answer decides which of them is the day and which is the year, and
// it reported no guess at all: strict mode accepted it and handed back
// 2010-05-01 as a certainty. A caller who kept the layout got 2001-05-10 out of
// the next row.
//
// The early return in textualDayIsAGuess is where it went. The loop walked the
// fields in input order and the two-digit-year arm returned before it reached
// the day, so the answer depended on which number the input wrote first, and
// the two halves of one question disagreed:
//
//	"01/02/10"  two interpretations, because the numbers are the question
//	"MAY10"     two interpretations, because the number could be either
//	"01MAY10"   accepted, and it is the same question with more evidence
//
// The instants below are the point. Both readings have to be real dates, nine
// years and nine days apart, or the caller has nothing to choose between: the
// alternative used to re-kind the day and leave the year alone, which describes
// an input holding two years and no day.
func TestStrictModeTwoNumbersCarriesBothReadings(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in        string
		dayLabel  string
		dayTime   string
		yearLabel string
		yearTime  string
	}{
		// The separator is not the point: the fuzzer found the run-together
		// form and the dashed and spaced forms read the same way.
		{"01MAY10", "DAY_MONTH_YEAR", "2010-05-01", "YEAR_MONTH_DAY", "2001-05-10"},
		{"01-MAY-10", "DAY_MONTH_YEAR", "2010-05-01", "YEAR_MONTH_DAY", "2001-05-10"},
		{"01 MAY 10", "DAY_MONTH_YEAR", "2010-05-01", "YEAR_MONTH_DAY", "2001-05-10"},
		{"15 MAY 20", "DAY_MONTH_YEAR", "2020-05-15", "YEAR_MONTH_DAY", "2015-05-20"},
		{"MAY15 20", "MONTH_DAY_YEAR", "2020-05-15", "MONTH_YEAR_DAY", "2015-05-20"},

		// The label is not the format name. classifyTextualPattern counts the
		// numbers and calls this MONTH_DAY_YEAR, and buildTextualFields then
		// reads the 10 as the year because it sits clear of the month name, so
		// the reading Detect chose is month, year, day. Naming it MONTH_DAY_YEAR
		// in the error would tell the caller the opposite of what they got.
		{"May 10, 24", "MONTH_YEAR_DAY", "2010-05-24", "MONTH_DAY_YEAR", "2024-05-10"},

		// RFC 822 and RFC 850, which is the loud half of this change. A
		// two-digit year is what both of them write, and nothing in the bytes
		// says the input is not "YY Mon DD": the weekday name in the RFC 850
		// form would settle it, and this library skips weekday names without
		// reading them. Strict mode refuses these now, and the reading it
		// chooses without strict mode has not moved.
		{"15 Mar 24 10:30 UTC", "DAY_MONTH_YEAR", "2024-03-15", "YEAR_MONTH_DAY", "2015-03-24"},
		{"Friday, 15-Mar-24 10:30:00 UTC", "DAY_MONTH_YEAR", "2024-03-15", "YEAR_MONTH_DAY", "2015-03-24"},
	}

	for _, c := range cases {
		opts := []Option{WithBaseTime(base), WithStrictMode(true)}
		_, err := ParseWith(c.in, opts...)
		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v, want *AmbiguousDateError", c.in, err, err)
			continue
		}
		got := map[string]string{}
		for _, i := range ade.Interpretations {
			got[i.Label] = i.Time.Format("2006-01-02")
		}
		if got[c.dayLabel] != c.dayTime || got[c.yearLabel] != c.yearTime || len(got) != 2 {
			t.Errorf("ParseWith(%q, strict) interpretations = %v\n"+
				"  want %s %s and %s %s: with a two-digit year already written, the two\n"+
				"  numbers swap, and both readings are dates the caller may have meant",
				c.in, got, c.dayLabel, c.dayTime, c.yearLabel, c.yearTime)
		}
	}
}

// TestTwoNumbersStayCertainWhenOneCannotBeADay is the other side of C27, and it
// is what stops the fix reporting a guess for every textual date.
//
// A number over 31 is not a day. It is the year under every reading, which
// leaves the number beside it the day under every reading, and there is nothing
// to choose between. A four-digit year says the same thing and says it of any
// value. Report a guess for these and the flag stops meaning anything: every
// RFC 2822 date in the world would carry it.
func TestTwoNumbersStayCertainWhenOneCannotBeADay(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want string
	}{
		{"70MAY1", "1970-05-01"},
		{"70MAY10", "1970-05-10"},
		{"70-MAY-01", "1970-05-01"},
		{"01MAY2010", "2010-05-01"},
		{"May 10, 2024", "2024-05-10"},
		{"Fri, 15 Mar 2024 10:30:00 +0000", "2024-03-15"},
		{"Mar 15 10:30:00 2024", "2024-03-15"},
		{"March 15th", "2026-03-15"},
	}

	for _, c := range cases {
		r, err := ParseWith(c.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q) = %v, want %s with no guess reported", c.in, err, c.want)
			continue
		}
		if got := r.Time.Format("2006-01-02"); got != c.want {
			t.Errorf("ParseWith(%q) = %s, want %s", c.in, got, c.want)
		}
		if r.Ambiguous {
			t.Errorf("ParseWith(%q) reports a guess; nothing in it could have been read "+
				"the other way", c.in)
		}
		if _, err := ParseWith(c.in, WithBaseTime(base), WithStrictMode(true)); err != nil {
			t.Errorf("ParseWith(%q, strict) = %v, want the same answer strict mode has "+
				"always given for an input that needed no guess", c.in, err)
		}
	}
}

// TestStrictModeInterpretationsAlwaysDiffer is the general property, asserted across
// every class of ambiguity rather than per input.
//
// Two identical interpretations is the bug C21 was filed for, and the assertion is
// what stops a fourth ambiguity class arriving with it: a caller cannot choose
// between two readings that are the same instant, and one who writes the obvious
// "both agree, so the guess was safe" guard is worse off than one who ignored the
// error.
//
// Every input below is one whose two readings are genuinely different dates. Two
// readings that coincide are not this bug and are covered by
// TestStrictModeReadingsMayCoincide; what makes them legitimate there is that both
// labels are true of the instant they carry, which is what fails here.
func TestStrictModeInterpretationsAlwaysDiffer(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in      string
		locales []Locale
		extra   []Option
	}{
		{in: "01/02/2024"},   // numeric field order
		{in: "01/02/03"},     // numeric field order, two-digit year
		{in: "01-02-03"},     // same, other separator
		{in: "03.02.2024"},   // same, and the dot heuristic decides the reading
		{in: "MAY 15"},       // textual day or two-digit year
		{in: "March 15"},     // same
		{in: "15 March"},     // same, number first
		{in: "MAY15"},        // same, no separator
		{in: "December 25"},  // same
		{in: "Mar 15 10:30"}, // same, with a time
		{in: "mai 15", locales: []Locale{FR}},
		{in: "15 mai", locales: []Locale{FR}},
		{in: "marzo 15", locales: []Locale{ES}},
		{in: "कल", locales: []Locale{HI}}, // a word with two meanings

		// The year's position, which is the one class that offers three
		// readings rather than two.
		{in: "01/02/03", extra: []Option{WithPreferYearFirst(true)}},
		{in: "25/12/01", extra: []Option{WithPreferYearFirst(true)}},
	}

	for _, c := range cases {
		opts := append([]Option{WithBaseTime(base), WithLocales(c.locales...), WithStrictMode(true)},
			c.extra...)
		_, err := ParseWith(c.in, opts...)
		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v, want *AmbiguousDateError", c.in, err, err)
			continue
		}
		if len(ade.Interpretations) < 2 {
			t.Errorf("ParseWith(%q, strict): %d interpretation(s); an ambiguous input has "+
				"at least two readings or it is not ambiguous", c.in, len(ade.Interpretations))
			continue
		}
		seenTime := map[int64]string{}
		seenLabel := map[string]bool{}
		for _, i := range ade.Interpretations {
			if prev, dup := seenTime[i.Time.UnixNano()]; dup {
				t.Errorf("ParseWith(%q, strict): interpretations %q and %q are both %s;\n"+
					"  the error has to carry the readings being chosen between, not two "+
					"copies of the chosen one", c.in, prev, i.Label, i.Time.Format(time.RFC3339))
			}
			seenTime[i.Time.UnixNano()] = i.Label
			if i.Label == "" {
				t.Errorf("ParseWith(%q, strict): an interpretation has no label", c.in)
			}
			if seenLabel[i.Label] {
				t.Errorf("ParseWith(%q, strict): two interpretations are labelled %q", c.in, i.Label)
			}
			seenLabel[i.Label] = true
			// The Layout is checked for existence and not for its name. A
			// natural-language reading carries the LayoutNaturalLanguage
			// sentinel, which refuses to re-parse on purpose, and labels the two
			// readings by date because the word that produced them is the same
			// word.
			if i.Layout == nil {
				t.Errorf("ParseWith(%q, strict): interpretation %q has no Layout", c.in, i.Label)
			}
		}
	}
}

// TestOrdinalDayIsNotAGuess is the other half of "the interpretations differ": an
// input has to have a second reading before it can be reported as ambiguous.
//
// "March 15th" was reported ambiguous and refused in strict mode. The suffix says
// the number is an ordinal, so there is no year reading to choose against, and the
// alternative offered for it would have been a fabrication: "2015th". Nothing else
// about the shape changes, so the flag was noise on an input with one reading.
func TestOrdinalDayIsNotAGuess(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want string
	}{
		{"March 15th", "2026-03-15"},
		{"MAY 15th", "2026-05-15"},
		{"15th March", "2026-03-15"},
		{"December 25th", "2026-12-25"},
		{"March 1st", "2026-03-01"},
		{"1st March", "2026-03-01"},
	}

	for _, c := range cases {
		r, err := ParseWith(c.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q) = %v", c.in, err)
			continue
		}
		if got := r.Time.Format("2006-01-02"); got != c.want {
			t.Errorf("ParseWith(%q) = %s, want %s", c.in, got, c.want)
		}
		if r.Ambiguous {
			t.Errorf("ParseWith(%q).Ambiguous = true; an ordinal suffix is a day and "+
				"nothing else, so there is no second reading to report", c.in)
		}
		if _, err := ParseWith(c.in, WithBaseTime(base), WithStrictMode(true)); err != nil {
			t.Errorf("ParseWith(%q, strict) = %v; strict mode refuses a guess, and "+
				"this input needs none", c.in, err)
		}
	}
}

// TestStrictModeLabelsTheReadingItCarries is the numeric half of C21 half one,
// which the card said worked.
//
// It did for a slash date and not for a dot date. The two readings were built by
// detecting the input twice with PreferDayFirst flipped, and resolveAmbiguousFields
// overrides that preference for a dot separator, so both detections came back
// day-first: "03.02.2024" produced the third of February twice, and one copy was
// labelled MM/DD/YYYY, which reads as the second of March. A caller filtering the
// interpretations for the American reading got a European one.
//
// The label now comes from the order the fields sit in, so it describes the reading
// it is attached to and cannot disagree with it.
func TestStrictModeLabelsTheReadingItCarries(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"03.02.2024", map[string]string{"DD/MM/YYYY": "2024-02-03", "MM/DD/YYYY": "2024-03-02"}},
		{"01.02.2024", map[string]string{"DD/MM/YYYY": "2024-02-01", "MM/DD/YYYY": "2024-01-02"}},
		{"01/02/2024", map[string]string{"MM/DD/YYYY": "2024-01-02", "DD/MM/YYYY": "2024-02-01"}},
		{"1/2/2024", map[string]string{"MM/DD/YYYY": "2024-01-02", "DD/MM/YYYY": "2024-02-01"}},
		// A two-digit year is named YY, because the label names the reading and
		// "01/02/03" is not a four-digit year.
		{"01/02/03", map[string]string{"MM/DD/YY": "2003-01-02", "DD/MM/YY": "2003-02-01"}},
		{"01-02-03", map[string]string{"MM/DD/YY": "2003-01-02", "DD/MM/YY": "2003-02-01"}},
		{"01/02/2024 10:30:00", map[string]string{"MM/DD/YYYY": "2024-01-02", "DD/MM/YYYY": "2024-02-01"}},
	}

	for _, c := range cases {
		// Both preferences, because which reading is chosen must not change what
		// either label means.
		for _, dayFirst := range []bool{false, true} {
			opts := []Option{WithBaseTime(base), WithPreferDayFirst(dayFirst), WithStrictMode(true)}
			_, err := ParseWith(c.in, opts...)
			var ade *AmbiguousDateError
			if !errors.As(err, &ade) {
				t.Errorf("ParseWith(%q, strict, dayFirst=%v) = %T %v, want *AmbiguousDateError",
					c.in, dayFirst, err, err)
				continue
			}
			got := map[string]string{}
			for _, i := range ade.Interpretations {
				got[i.Label] = i.Time.Format("2006-01-02")
			}
			if len(got) != len(c.want) {
				t.Errorf("ParseWith(%q, strict, dayFirst=%v) = %v, want %v", c.in, dayFirst, got, c.want)
				continue
			}
			for label, want := range c.want {
				if got[label] != want {
					t.Errorf("ParseWith(%q, strict, dayFirst=%v): %s = %q, want %q\n"+
						"  a label names the reading it is attached to, or a caller picking by "+
						"label gets the other one", c.in, dayFirst, label, got[label], want)
				}
			}
		}
	}
}

// TestStrictModeReadingsMayCoincide is the case that looks like the C21 bug and is
// not one.
//
// "01/01/2024" is ambiguous: the parser had to decide which 01 was the month, and
// result.Ambiguous says so. Both decisions land on the first of January, and each
// label is true of the instant beside it. That is worth pinning, because the
// tempting fix for C21 is to drop a reading that duplicates another, and dropping
// one here would leave a single-reading error on an input that genuinely has two.
func TestStrictModeReadingsMayCoincide(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{"01/01/2024", "02/02/2024", "12/12/2024"} {
		_, err := ParseWith(in, WithBaseTime(base), WithStrictMode(true))
		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v, want *AmbiguousDateError", in, err, err)
			continue
		}
		if len(ade.Interpretations) != 2 {
			t.Errorf("ParseWith(%q, strict): %d interpretations, want 2", in, len(ade.Interpretations))
			continue
		}
		if !ade.Interpretations[0].Time.Equal(ade.Interpretations[1].Time) {
			t.Errorf("ParseWith(%q, strict): %v and %v differ; the two readings of a date "+
				"whose parts are equal are the same date", in,
				ade.Interpretations[0].Time, ade.Interpretations[1].Time)
		}
	}
}

// TestStrictModeNeedsTwoReadings covers the inputs detection calls ambiguous and
// execution then disagrees with.
//
// "March 00" is read as a day, because 00 is not over 31, and there is no
// zeroth of March. Only the year reading parses, and an *AmbiguousDateError
// carrying it alone would hand strict mode's caller 2000-03-01 for an input that
// Parse refuses outright: strict mode is allowed to refuse more than the lenient
// path and never to accept more.
func TestStrictModeNeedsTwoReadings(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{"March 00", "MAY00", "00 March", "15 March 10:30"} {
		if _, err := ParseWith(in, WithBaseTime(base)); err == nil {
			t.Errorf("test premise gone: ParseWith(%q) now parses", in)
			continue
		}
		_, err := ParseWith(in, WithBaseTime(base), WithStrictMode(true))
		var ade *AmbiguousDateError
		if errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = *AmbiguousDateError with %d interpretation(s); "+
				"an input the lenient path refuses has no readings to choose between",
				in, len(ade.Interpretations))
			continue
		}
		if !errors.Is(err, ErrAmbiguous) {
			t.Errorf("ParseWith(%q, strict) = %v, want an error unwrapping to ErrAmbiguous", in, err)
		}
	}
}

// TestStrictModeYearFirstCarriesEveryReading is C22's half of the strict-mode
// question, which the card left open deliberately: a year-first input has three
// honest readings where every other ambiguous input has two, and what the error
// carries had to be decided rather than fall out.
//
// It carries all three. Strict mode exists to hand back what the library was
// choosing between, and the preference chose the year-first reading against
// both year-last orders, not against one of them.
//
// What it does not carry is YY/DD/MM. A format that writes the year first
// writes ISO order after it, so the reading the option enables is year, month,
// day, and the month-versus-day question does not arise inside it.
func TestStrictModeYearFirstCarriesEveryReading(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		in   string
		want map[string]string
	}{
		{"01/02/03", map[string]string{
			"YY/MM/DD": "2001-02-03",
			"MM/DD/YY": "2003-01-02",
			"DD/MM/YY": "2003-02-01",
		}},
		{"01-02-03", map[string]string{
			"YY/MM/DD": "2001-02-03",
			"MM/DD/YY": "2003-01-02",
			"DD/MM/YY": "2003-02-01",
		}},
		// 25 is not a month, so the year-last reading that would need it to be
		// one is not offered: a reading that cannot parse the input is not one
		// of the ones the caller is being asked about.
		{"25/12/01", map[string]string{
			"YY/MM/DD": "2025-12-01",
			"DD/MM/YY": "2001-12-25",
		}},
	} {
		_, err := ParseWith(tt.in, WithBaseTime(base), WithPreferYearFirst(true), WithStrictMode(true))
		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, yearFirst, strict) = %T %v, want *AmbiguousDateError", tt.in, err, err)
			continue
		}
		got := map[string]string{}
		for _, i := range ade.Interpretations {
			got[i.Label] = i.Time.Format("2006-01-02")
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParseWith(%q, yearFirst, strict) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for label, want := range tt.want {
			if got[label] != want {
				t.Errorf("ParseWith(%q, yearFirst, strict): %s = %q, want %q", tt.in, label, got[label], want)
			}
		}
		// The reading the preference chose comes first, so a caller who takes
		// Interpretations[0] takes what the lenient path would have returned.
		lenient, lerr := ParseWith(tt.in, WithBaseTime(base), WithPreferYearFirst(true))
		if lerr != nil {
			t.Errorf("ParseWith(%q, yearFirst): %v", tt.in, lerr)
			continue
		}
		if !ade.Interpretations[0].Time.Equal(lenient.Time) {
			t.Errorf("ParseWith(%q, yearFirst, strict): first interpretation is %v, but the "+
				"lenient path returns %v", tt.in, ade.Interpretations[0].Time, lenient.Time)
		}
	}
}
