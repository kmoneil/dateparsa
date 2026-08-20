package dateparsa

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParse_ISO8601(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		layout   string
	}{
		{
			name:     "ISO date",
			input:    "2024-03-15",
			expected: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			layout:   "ISO8601_DATE",
		},
		{
			name:     "ISO datetime",
			input:    "2024-03-15T10:30:00",
			expected: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
			layout:   "ISO8601_DATETIME",
		},
		{
			name:     "ISO datetime with Z",
			input:    "2024-03-15T10:30:00Z",
			expected: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
			layout:   "ISO8601_DATETIME_Z",
		},
		{
			name:     "RFC3339 with timezone",
			input:    "2024-03-15T10:30:00+05:30",
			expected: time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+05:30", 5*3600+30*60)),
			layout:   "RFC3339",
		},
		{
			name:     "RFC3339 negative timezone",
			input:    "2024-03-15T10:30:00-08:00",
			expected: time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("-08:00", -8*3600)),
			layout:   "RFC3339",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
			if result.Layout.String() != tt.layout {
				t.Errorf("Parse(%q) layout = %q, want %q", tt.input, result.Layout.String(), tt.layout)
			}
			if result.Ambiguous {
				t.Errorf("Parse(%q) should not be ambiguous", tt.input)
			}
			if result.Kind != KindAbsolute {
				t.Errorf("Parse(%q) kind = %v, want KindAbsolute", tt.input, result.Kind)
			}
		})
	}
}

func TestParse_SQLDatetime(t *testing.T) {
	result, err := Parse("2024-03-15 10:30:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
	if result.Layout.String() != "SQL_DATETIME" {
		t.Errorf("layout = %q, want SQL_DATETIME", result.Layout.String())
	}
}

func TestParse_EuropeanDot(t *testing.T) {
	result, err := Parse("15.03.2024")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_TimeOnly(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		hour   int
		minute int
		second int
	}{
		{"HH:MM", "10:30", 10, 30, 0},
		{"HH:MM:SS", "10:30:45", 10, 30, 45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if result.Time.Hour() != tt.hour || result.Time.Minute() != tt.minute || result.Time.Second() != tt.second {
				t.Errorf("Parse(%q) time = %02d:%02d:%02d, want %02d:%02d:%02d",
					tt.input,
					result.Time.Hour(), result.Time.Minute(), result.Time.Second(),
					tt.hour, tt.minute, tt.second)
			}
		})
	}
}

func TestParse_TextualMonth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			name:     "March 15, 2024",
			input:    "March 15, 2024",
			expected: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Mar 15, 2024",
			input:    "Mar 15, 2024",
			expected: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "15 March 2024",
			input:    "15 March 2024",
			expected: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "15 Mar 2024",
			input:    "15 Mar 2024",
			expected: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "January 1, 2000",
			input:    "January 1, 2000",
			expected: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "December 31, 1999",
			input:    "December 31, 1999",
			expected: time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_AmbiguousNumeric(t *testing.T) {
	// 01/02/2024 is ambiguous: could be Jan 2 or Feb 1.
	t.Run("default (MDY)", func(t *testing.T) {
		result, err := Parse("01/02/2024")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Default: MM/DD/YYYY
		expected := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		if !result.Time.Equal(expected) {
			t.Errorf("got %v, want %v", result.Time, expected)
		}
		if !result.Ambiguous {
			t.Error("expected Ambiguous=true")
		}
	})

	t.Run("prefer day first (DMY)", func(t *testing.T) {
		result, err := ParseWith("01/02/2024", WithPreferDayFirst(true))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// DD/MM/YYYY
		expected := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		if !result.Time.Equal(expected) {
			t.Errorf("got %v, want %v", result.Time, expected)
		}
	})

	t.Run("unambiguous: 13/01/2024 must be DMY", func(t *testing.T) {
		result, err := Parse("13/01/2024")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC)
		if !result.Time.Equal(expected) {
			t.Errorf("got %v, want %v", result.Time, expected)
		}
		if result.Ambiguous {
			t.Error("13/01/2024 should NOT be ambiguous")
		}
	})
}

func TestParse_StrictMode(t *testing.T) {
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	if err == nil {
		t.Fatal("expected error in strict mode for ambiguous date")
	}
	ambigErr, ok := err.(*AmbiguousDateError)
	if !ok {
		t.Fatalf("expected *AmbiguousDateError, got %T: %v", err, err)
	}
	if len(ambigErr.Interpretations) < 2 {
		t.Errorf("expected at least 2 interpretations, got %d", len(ambigErr.Interpretations))
	}
}

func TestParse_InvalidInput(t *testing.T) {
	inputs := []string{
		"",
		"not a date",
		"foo bar baz",
		"!@#$%^&*()",
	}
	for _, input := range inputs {
		_, err := Parse(input)
		if err == nil {
			t.Errorf("Parse(%q) should have returned an error", input)
		}
	}
}

func TestLayout_Reuse(t *testing.T) {
	// Parse once to detect the layout.
	result, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reuse the layout for a different date.
	t2, err := result.Layout.Parse("2025-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Layout.Parse error: %v", err)
	}
	expected := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if !t2.Equal(expected) {
		t.Errorf("Layout.Parse = %v, want %v", t2, expected)
	}
}

func TestLayout_GoLayout(t *testing.T) {
	result, err := Parse("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	goLayout, ok := result.Layout.GoLayout()
	if !ok {
		t.Fatal("expected GoLayout to return true for ISO date")
	}
	if goLayout != "2006-01-02" {
		t.Errorf("GoLayout = %q, want %q", goLayout, "2006-01-02")
	}
}

func TestDetect(t *testing.T) {
	layout, err := Detect("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if layout.String() != "ISO8601_DATETIME_Z" {
		t.Errorf("Detect layout = %q, want ISO8601_DATETIME_Z", layout.String())
	}
}

func TestParser_BatchParsing(t *testing.T) {
	p := NewParser()

	// Parse a column of ISO dates.
	values := []string{
		"2024-01-01",
		"2024-06-15",
		"",
		"2024-12-31",
	}

	times, errs := p.ParseColumn(values)

	for i, v := range values {
		if v == "" {
			if errs[i] != nil {
				t.Errorf("row %d: expected nil error for empty, got %v", i, errs[i])
			}
			continue
		}
		if errs[i] != nil {
			t.Errorf("row %d (%q): unexpected error: %v", i, v, errs[i])
			continue
		}
		if times[i].IsZero() {
			t.Errorf("row %d (%q): got zero time", i, v)
		}
	}

	if !times[0].Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("row 0: got %v", times[0])
	}
	if !times[1].Equal(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("row 1: got %v", times[1])
	}
}

func TestParseTime(t *testing.T) {
	tt, err := ParseTime("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if tt.Year() != 2024 || tt.Month() != 3 || tt.Day() != 15 {
		t.Errorf("got %v", tt)
	}
}

func TestParse_FillsCurrentYear(t *testing.T) {
	// Parse without options should fill in the current year for time-only inputs.
	result, err := Parse("10:30")
	if err != nil {
		t.Fatalf("Parse(\"10:30\") error: %v", err)
	}
	if result.Time.Year() != time.Now().Year() {
		t.Errorf("Parse(\"10:30\") year = %d, want %d", result.Time.Year(), time.Now().Year())
	}
}

func TestLayout_Parse_ReturnsParseError(t *testing.T) {
	// Detect a layout, then use it on invalid input.
	result, err := Parse("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	_, err = result.Layout.Parse("not-valid!")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("Layout.Parse error should be *ParseError, got %T: %v", err, err)
	}
	if pe.Input != "not-valid!" {
		t.Errorf("ParseError.Input = %q, want %q", pe.Input, "not-valid!")
	}
}

func TestParse_RFC3339Nano(t *testing.T) {
	result, err := Parse("2024-03-15T10:30:00.123456789Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 123456789, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_EpochLayout(t *testing.T) {
	result, err := Parse("1710504800")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Layout != LayoutEpoch {
		t.Errorf("Layout = %v, want LayoutEpoch", result.Layout)
	}
	if result.Layout.String() != "UNIX_TIMESTAMP" {
		t.Errorf("Layout.String() = %q, want UNIX_TIMESTAMP", result.Layout.String())
	}
	if _, ok := result.Layout.GoLayout(); ok {
		t.Error("LayoutEpoch.GoLayout() should return false")
	}
	// Sentinel layout should return a clear error on re-parse.
	_, err = result.Layout.Parse("1710504800")
	if err == nil {
		t.Fatal("LayoutEpoch.Parse should return error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("expected *ParseError, got %T", err)
	}
}

func TestParse_NLLayout(t *testing.T) {
	result, err := ParseWith("3 days ago", WithBaseTime(time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Layout != LayoutNaturalLanguage {
		t.Errorf("Layout = %v, want LayoutNaturalLanguage", result.Layout)
	}
	if result.Layout.String() != "NATURAL_LANGUAGE" {
		t.Errorf("Layout.String() = %q, want NATURAL_LANGUAGE", result.Layout.String())
	}
	// Sentinel layout should return a clear error on re-parse.
	_, err = result.Layout.Parse("3 days ago")
	if err == nil {
		t.Fatal("LayoutNaturalLanguage.Parse should return error")
	}
}

// TestLayoutReusableAgreesWithParse is N9.1's half that is a change to the API
// rather than to a document, amended by C27's second half.
//
// Reusable answers the question every caller of this library has after a parse,
// and until N9.1 there was no way to ask it. The comparison benchmark, which is
// a separate module and therefore an ordinary caller, worked it out by calling
// Parse("") and string-comparing Layout.String() against the two sentinel
// labels. In this package the fuzz target compared against the two sentinels by
// identity, which a third sentinel would have broken silently.
//
// The property was "Reusable is true exactly when re-parsing works", and that
// is the narrower question the name does not ask. C27 is the input where the
// two come apart: a layout from "70MAY1" re-parses "01MAY10" perfectly well and
// answers 2001-05-10 where Parse answers 2010-05-01. Re-parsing working is
// necessary and is not sufficient.
//
// So the property is now two-sided, and false has two meanings the test states
// separately: a sentinel refuses to parse at all, and a prone layout parses and
// may be wrong. Nothing else may be false.
func TestLayoutReusableAgreesWithParse(t *testing.T) {
	base := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in       string
		reusable bool
		why      string
	}{
		{"2024-03-15", true, "shape decides every field"},
		{"2024-03-15T10:30:00Z", true, ""},
		{"2024-03-15T10:30:00+05:53", true, "off the pre-built zone grid"},
		{"March 15, 2024", true, "a four-digit year cannot be a day"},
		{"10:30:00", true, ""},
		{"15 March 2024", true, ""},

		// Prone: detection read the values, so the next row may want the other
		// reading and the program cannot tell.
		{"01/02/2024", false, "DD/MM against MM/DD"},
		{"25/12/2024", false, "unambiguous itself, and its layout meets 01/02"},
		{"70MAY1", false, "C27: reads 01MAY10 as 2001-05-10"},
		{"MAY70", false, "reads MAY10 as 2010-05-01"},

		// Sentinels: no program at all.
		{"1710504800", false, "epoch"},
		{"1710504800000", false, "epoch in milliseconds"},
		{"3 days ago", false, "natural language"},
		{"now", false, ""},
		{"yesterday at 5pm", false, ""},
	}

	for _, c := range cases {
		result, err := ParseWith(c.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.in, err)
			continue
		}
		l := result.Layout
		if got := l.Reusable(); got != c.reusable {
			t.Errorf("ParseWith(%q).Layout.Reusable() = %v, want %v (%s)",
				c.in, got, c.reusable, c.why)
			continue
		}

		_, perr := l.Parse(c.in)
		if c.reusable && perr != nil {
			t.Errorf("ParseWith(%q).Layout: Reusable() but re-parsing the same input "+
				"fails: %v", c.in, perr)
			continue
		}
		if c.reusable {
			continue
		}

		// The two meanings of false, and which one this is has to be legible
		// from the value rather than from the table: a sentinel carries no
		// program and refuses, and a prone layout parses.
		switch {
		case !l.hasProgram():
			if perr == nil {
				t.Errorf("ParseWith(%q).Layout has no program but re-parses", c.in)
			}
		case !l.ambiguityProne:
			t.Errorf("ParseWith(%q).Layout.Reusable() = false and the layout is neither "+
				"a sentinel nor ambiguity-prone; those are the only two reasons", c.in)
		case perr != nil:
			t.Errorf("ParseWith(%q).Layout is prone and refuses its own input: %v\n"+
				"  a prone layout parses and may answer the wrong day, which is what "+
				"makes it different from a sentinel", c.in, perr)
		}
	}

	// The check a caller makes before using what they were handed, so it has to
	// survive the value being nil rather than being the panic it is guarding.
	var nilLayout *Layout
	if nilLayout.Reusable() {
		t.Error("(*Layout)(nil).Reusable() = true")
	}
	if nilLayout.hasProgram() {
		t.Error("(*Layout)(nil).hasProgram() = true")
	}
}

// TestReusableSaysNoToALayoutParserWillNotReuse is C27's second half, and it
// replaces TestReusableSaysYesToALayoutParserWillNotReuse, which pinned the
// hazard and was written to fail when this landed.
//
// Reusable was l != nil && l.program.N != 0, so it answered "this is not a
// sentinel" while its doc comment offered it as the check a caller makes before
// keeping a layout, and README.md showed exactly that. Parser answered a second
// question the caller could not ask: an ambiguity-prone layout is one whose
// fields were decided by looking at the values, so it accepts the next row
// whichever way that row wanted to be read, and Parser declines its own cache
// on one. A caller holding the same layout was told yes.
//
// Both halves of the hazard are here. C27 is the textual one, found by the
// fuzzer; the numeric one is older and is the same shape, which is the argument
// for answering it in Reusable rather than in a second method: one question,
// one answer, every format.
//
// What did not change is the fast path. A prone layout still parses, and it
// still answers the first row's way, because a caller who knows their column is
// uniform is entitled to that. The method says the column has to be known.
func TestReusableSaysNoToALayoutParserWillNotReuse(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		seed, next     string
		reused, detect string
	}{
		{"70MAY1", "01MAY10", "2001-05-10", "2010-05-01"},
		{"70MAY10", "01MAY10", "2001-05-10", "2010-05-01"},
		{"70-MAY-01", "01-MAY-10", "2001-05-10", "2010-05-01"},
		{"MAY70", "MAY10", "2010-05-01", "2026-05-10"},
		{"25/12/2024", "01/02/2024", "2024-02-01", "2024-01-02"},
	}

	for _, c := range cases {
		seed, err := ParseWith(c.seed, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.seed, err)
			continue
		}

		if seed.Layout.Reusable() {
			t.Errorf("ParseWith(%q).Layout.Reusable() = true: the layout answers %s for "+
				"%q where detection answers %s, so a caller who keeps it on this "+
				"gets the wrong day with a nil error",
				c.seed, c.reused, c.next, c.detect)
			continue
		}

		// It is not a sentinel, which is the other reason Reusable says no, and
		// the difference is the whole point: this one parses.
		if !seed.Layout.hasProgram() {
			t.Errorf("ParseWith(%q).Layout has no program; this layout is prone, not "+
				"a sentinel", c.seed)
			continue
		}
		got, err := seed.Layout.Parse(c.next)
		if err != nil {
			t.Errorf("layout from %q on %q: %v; a prone layout still parses, and that "+
				"is why Reusable had to be the thing that says no", c.seed, c.next, err)
			continue
		}
		if got.Format("2006-01-02") != c.reused {
			t.Errorf("layout from %q on %q = %s, want %s: the hazard this method "+
				"reports has moved and the test no longer describes it",
				c.seed, c.next, got.Format("2006-01-02"), c.reused)
		}

		// And detection, which is what the caller gets by not reusing.
		fresh, err := ParseWith(c.next, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.next, err)
			continue
		}
		if fresh.Time.Format("2006-01-02") != c.detect {
			t.Errorf("ParseWith(%q) = %s, want %s", c.next,
				fresh.Time.Format("2006-01-02"), c.detect)
		}

		// Parser is the answer a caller should be given, and it was always
		// right: it re-detects rather than reusing, so the column comes back
		// row by row.
		p := NewParser(WithBaseTime(base))
		for _, in := range []struct{ s, want string }{
			{c.seed, seed.Time.Format("2006-01-02")},
			{c.next, c.detect},
			{c.seed, seed.Time.Format("2006-01-02")},
		} {
			r, err := p.Parse(in.s)
			if err != nil {
				t.Errorf("Parser.Parse(%q): %v", in.s, err)
				continue
			}
			if got := r.Time.Format("2006-01-02"); got != in.want {
				t.Errorf("Parser.Parse(%q) after %q = %s, want %s",
					in.s, c.seed, got, in.want)
			}
		}
	}
}

// TestReusableStillSaysYesToAShapeDecidedFormat is the other side of it, and it
// is what stops the fix reading as "reuse is off".
//
// The whole reason this library exists is that detection is reusable, so a
// method that says no to a format whose fields are fixed by its shape would
// have taken that away to close a hole in the formats where they are not. Every
// input here is one whose layout carries the answer for any row that parses
// against it.
//
// "13/01/2024" is deliberately not here, and it is the input that shows what
// prone means. It needed no guess, because 13 cannot be a month, and the layout
// it yields is the same program as the one from "01/02/2024": ambiguityProne is
// a property of the format and not of the row it was detected from, which is
// the whole reason a cached answer cannot be trusted for the next row. So
// Reusable says no to the numeric slash family whatever value detected it. That
// is the visible cost of C27's second half and it is the same hazard, found
// earlier and never closed on this path.
func TestReusableStillSaysYesToAShapeDecidedFormat(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"2024-03-15 10:30:00",
		"20240315",
		"March 15, 2024",
		"15 March 2024",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Fri Mar 15 10:30:00 2024",
		"10:30:45",
		"2024-03-15T10:30:00+05:53",
	} {
		result, err := ParseWith(in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", in, err)
			continue
		}
		if !result.Layout.Reusable() {
			t.Errorf("ParseWith(%q).Layout.Reusable() = false: this format's fields are "+
				"decided by its shape, and reuse on those is what the library is for", in)
		}
	}
}

// TestLayoutRefusesTrailingInput pins the property Parser depends on: a layout
// describes a whole input or refuses it. Without this, a cached ISO8601_DATE
// accepted "2024-03-16T10:30:00Z" and returned midnight with no error, so one
// date-only row at the top of a CSV column silently stripped the time of day
// and the timezone from every row under it.
func TestLayoutRefusesTrailingInput(t *testing.T) {
	base, err := Parse("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{
		"2024-03-15T10:30:00Z",
		"2024-03-15 10:30:00",
		"2024-03-15 is when it happened",
		"2024-03-1500000",
	} {
		if got, err := base.Layout.Parse(in); err == nil {
			t.Errorf("ISO8601_DATE.Parse(%q) = %v, want an error", in, got)
		}
	}

	// And the column that motivated it: every row agrees with fresh detection.
	p := NewParser()
	col := []string{"2024-03-15", "2024-03-16T10:30:00Z", "2024-03-17T23:59:59+05:30"}
	times, errs := p.ParseColumn(col)
	for i, in := range col {
		if errs[i] != nil {
			t.Fatalf("row %d %q: %v", i, in, errs[i])
		}
		fresh, err := Parse(in)
		if err != nil {
			t.Fatalf("row %d %q: %v", i, in, err)
		}
		if !times[i].Equal(fresh.Time) {
			t.Errorf("row %d %q: cached layout gave %v, detection gives %v", i, in, times[i], fresh.Time)
		}
	}
}

// TestZoneOffsetIsNotDropped covers two formats that carried an offset the
// detector read past, each returning an instant in the wrong timezone with no
// error. Neither is visible to a test that checks only the wall-clock fields,
// which is why both survived: the hour is right in both, and the instant is
// not.
func TestZoneOffsetIsNotDropped(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			// Go's own time.Time.String() for a non-UTC zone. The fractional
			// variant went to ISO8601_FRAC, which stopped at the fraction and
			// left the value reading as UTC, three hours off. The same value
			// without a fraction reached detectGoTimeString and came out right,
			// so adding a fractional second changed the answer by three hours.
			name:  "Go time.String with fraction and offset",
			input: "2012-08-03 18:31:59.257000000 +0300 MSK",
			want:  time.Date(2012, 8, 3, 18, 31, 59, 257000000, time.FixedZone("MSK", 3*3600)),
		},
		{
			name:  "Go time.String without fraction",
			input: "2015-02-08 03:02:00 +0300 MSK",
			want:  time.Date(2015, 2, 8, 3, 2, 0, 0, time.FixedZone("MSK", 3*3600)),
		},
		{
			// JavaScript Date.toString(). The zone is a name and an offset
			// together; reading only the name gave UTC for a value an hour
			// ahead of it.
			name:  "JS Date.toString GMT+0100",
			input: "Fri Jul 03 2015 18:04:07 GMT+0100",
			want:  time.Date(2015, 7, 3, 18, 4, 7, 0, time.FixedZone("", 3600)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}
			if !got.Time.Equal(tt.want) {
				t.Errorf("Parse(%q) = %v (unix %d), want %v (unix %d), off by %v",
					tt.input, got.Time, got.Time.Unix(), tt.want, tt.want.Unix(),
					got.Time.Sub(tt.want))
			}
		})
	}
}

// TestStrictModeSurvivesTheParserCache pins the half of strict mode that the
// layout cache used to take away. A Parser seeded with an unambiguous value of
// an ambiguity-prone format cached a layout, and every later row went through
// it without the ambiguity check, so a caller who asked to be told about
// guesses stopped being told after the first success.
//
// The gate is on whether the format can need a guess, not on whether the parse
// that produced the layout did. Ambiguity belongs to the input: "25/12/2024"
// resolves without one and still yields a layout that meets "01/02/2024" two
// rows later.
func TestStrictModeSurvivesTheParserCache(t *testing.T) {
	seeds := []struct{ unambiguous, ambiguous string }{
		{"25/12/2024", "01/02/2024"}, // 25 cannot be a month
		{"March 5", "March 15"},      // one digit cannot be a year
	}
	for _, tt := range seeds {
		p := NewParser(WithStrictMode(true))
		if _, err := p.Parse(tt.unambiguous); err != nil {
			t.Fatalf("seed %q: %v", tt.unambiguous, err)
		}
		_, err := p.Parse(tt.ambiguous)
		if !errors.Is(err, ErrAmbiguous) {
			t.Errorf("after caching %q, Parse(%q) = %v, want ErrAmbiguous",
				tt.unambiguous, tt.ambiguous, err)
		}
		var ade *AmbiguousDateError
		if errors.As(err, &ade) && len(ade.Interpretations) < 2 {
			t.Errorf("Parse(%q): %d interpretations, want both", tt.ambiguous, len(ade.Interpretations))
		}
	}

	// Strict mode must not turn into a blanket refusal.
	p := NewParser(WithStrictMode(true))
	for _, s := range []string{"2024-03-15", "25/12/2024", "March 2015", "MAY70", "March 15, 2024"} {
		if _, err := p.Parse(s); err != nil {
			t.Errorf("Parse(%q) refused in strict mode: %v", s, err)
		}
	}
}

// TestParserReportsAmbiguityLikeParse pins the whole of the property: a row read
// through a Parser and the same row read through Parse agree on the instant and
// on the flag, whatever seeded the cache.
//
// It began as three rows after one seed, and every one of those rows was
// ambiguous exactly like the seed, so the samples could not reach what the
// assertion was for. Twenty-one inputs diverged behind it, in two families:
//
//	seed        row         Parser              Parse
//	03/15/2024  01/02/2024  2024-01-02 false    2024-01-02 true     flag
//	01/02/2024  03/15/2024  2024-03-15 true     2024-03-15 false    flag
//	25/12/2024  01/02/2024  2024-02-01 false    2024-01-02 true     INSTANT
//	MAY70       MAY10       2010-05-01 false    2026-05-10 true     INSTANT
//	March 32    March 31    2031-03-01 false    2026-03-31 true     INSTANT
//
// Both families have the same root. Detection resolved the format by looking at
// the values, and the readings it chose between emit the same fields at the same
// offsets and widths, so the cached program parses the next row whichever way
// that row wanted to be read. A value-derived field assignment cannot be reused,
// only re-derived, which is what Parser does now for any layout detection marks
// ambiguity-prone.
//
// The seeds below are chosen so that the seed and the row resolve differently.
// A table where they agree passes against the unfixed tree.
func TestParserReportsAmbiguityLikeParse(t *testing.T) {
	seqs := []struct {
		name string
		seed string
		rows []string
	}{
		// Numeric: which part is the month is decided by value, and the
		// preference decides only when both parts could be either.
		{"slash/unambiguous seed", "03/15/2024", []string{"01/02/2024", "05/06/2024"}},
		{"slash/ambiguous seed", "01/02/2024", []string{"03/15/2024", "05/06/2024"}},
		{"slash/day-first seed", "25/12/2024", []string{"01/02/2024", "05/06/2024"}},
		{"slash/2-digit year", "03/15/24", []string{"01/02/24"}},
		{"dot/day-first", "15.03.2024", []string{"01.02.2024"}},
		{"dash", "03-15-2024", []string{"01-02-2024"}},

		// Textual: a bare number over 31 is a year and at or under 31 is a day,
		// and both are two bytes at the same offset.
		{"textual/year seed", "MAY70", []string{"MAY10", "MAY99"}},
		{"textual/day seed", "MAY10", []string{"MAY70"}},
		{"textual/spaced", "March 32", []string{"March 31", "March 15"}},
		{"textual/day first", "70 March", []string{"10 March"}},
		{"textual/abbrev", "Mar 70", []string{"Mar 10"}},
		{"textual/width settles it", "March 5", []string{"March 15"}},
		{"textual/4-digit year settles it", "March 2015", []string{"March 15"}},
		// An ordinal suffix settles it too, and that one keeps the cache: the
		// layout accepts nothing without the suffix, and nothing with it is a
		// year. This is the pair that has to agree for that to be sound.
		{"textual/ordinal settles it", "March 5th", []string{"March 15th", "March 20th"}},
		{"textual/ordinal, wide seed", "March 15th", []string{"March 20th", "March 5th"}},

		// A shaped format is not prone and must keep using the cache.
		{"shaped", "2024-03-15T10:30:00Z", []string{"2025-01-01T00:00:00Z"}},
		{"format changes mid-column", "01/02/2024", []string{"2024-03-15"}},
	}

	for _, seq := range seqs {
		t.Run(seq.name, func(t *testing.T) {
			p := NewParser()
			if _, err := p.Parse(seq.seed); err != nil {
				t.Fatalf("seed %q: %v", seq.seed, err)
			}
			for _, s := range seq.rows {
				cached, cerr := p.Parse(s)
				fresh, ferr := Parse(s)
				if (cerr == nil) != (ferr == nil) {
					t.Fatalf("after %q, %q: Parser err=%v, Parse err=%v",
						seq.seed, s, cerr, ferr)
				}
				if cerr != nil {
					continue
				}
				if !cached.Time.Equal(fresh.Time) {
					t.Errorf("after %q, %q: Parser returns %v, Parse returns %v",
						seq.seed, s, cached.Time, fresh.Time)
				}
				if cached.Ambiguous != fresh.Ambiguous {
					t.Errorf("after %q, %q: Parser reports Ambiguous=%v, Parse reports %v",
						seq.seed, s, cached.Ambiguous, fresh.Ambiguous)
				}
			}
		})
	}
}

// TestParserConcurrentUse is the gate on the promise in Parser's doc comment.
// The cached layout lives in an atomic pointer and the Layout behind it is
// immutable, so several goroutines may share one Parser.
//
// It is a race-detector test first and a correctness test second. On the commit
// before the atomic pointer it reports a write/read data race on Parser.layout
// under -race, and it is in the suite that runs with -race in CI for that
// reason. The correctness half matters too: each goroutine parses a different
// format through the same cache, so a goroutine that picked up another's layout
// and trusted it would return the wrong instant rather than falling through to
// detection.
func TestParserConcurrentUse(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2024-03-15", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024-03-15T10:30:00Z", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"20240315", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024-03-15 10:30:00", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"March 15, 2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}

	p := NewParser()
	var wg sync.WaitGroup
	for g := range 12 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			c := cases[g%len(cases)]
			for range 500 {
				got, err := p.Parse(c.in)
				if err != nil {
					t.Errorf("goroutine %d: Parse(%q): %v", g, c.in, err)
					return
				}
				if !got.Time.Equal(c.want) {
					t.Errorf("goroutine %d: Parse(%q) = %v, want %v", g, c.in, got.Time, c.want)
					return
				}
			}
		}(g)
	}

	// Reset races the readers on purpose: it is documented as safe to call
	// while other goroutines parse, and nothing else in the suite calls it
	// concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			p.Reset()
		}
	}()

	wg.Wait()
}

// TestParserKeepsTheCacheForShapedFormats is the other half of the gate above.
// Declining the cache is only correct where the format was resolved by value; a
// format whose fields are fixed by its shape must still skip detection, or the
// fix for the ambiguity-prone case has quietly turned Parser into Parse.
func TestParserKeepsTheCacheForShapedFormats(t *testing.T) {
	for _, s := range []string{
		"2024-03-15T10:30:00Z",
		"2024-03-15",
		"2024-03-15 10:30:00",
		"March 15, 2024",
		"20240315",
	} {
		p := NewParser()
		if _, err := p.Parse(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
		before := p.layout.Load()
		if before == nil {
			t.Fatalf("%q: no layout cached", s)
		}
		allocs := testing.AllocsPerRun(100, func() {
			_, _ = p.Parse(s)
		})
		if allocs > 0 {
			t.Errorf("%q: Parser.Parse allocated %.0f times on a cache hit, want 0"+
				" (ambiguityProne=%v)", s, allocs, before.ambiguityProne)
		}
	}
}

// TestBareMonthNumberReportsTheGuess covers the value-dependent classification
// of a month and a bare number. Over 31 it must be a year, one digit cannot be
// a year, and anything else is a choice the caller has to be told about:
// "MAY70" is May 1970 and "MAY10" is the tenth of May, and until this was
// flagged nothing said the second was a guess.
func TestBareMonthNumberReportsTheGuess(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"March 5", false},        // one digit, no year is written with one
		{"March 05", true},        // two digits at or under 31
		{"March 15", true},        //
		{"March 31", true},        //
		{"MAY10", true},           // no separator, same question
		{"15 March", true},        // and the same before the month
		{"5 March", false},        //
		{"March 32", false},       // over 31, so not a day
		{"MAY70", false},          //
		{"March 15th", false},     // an ordinal suffix is a day, never a year
		{"15th March", false},     //
		{"March 2015", false},     // four digits
		{"March 15, 2024", false}, // a year is written out
		{"2024-03-15", false},     // no question arises
	}
	for _, tt := range tests {
		got, err := Parse(tt.input)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.input, err)
		}
		if got.Ambiguous != tt.want {
			t.Errorf("Parse(%q).Ambiguous = %v, want %v", tt.input, got.Ambiguous, tt.want)
		}
		_, err = ParseWith(tt.input, WithStrictMode(true))
		if (err != nil) != tt.want {
			t.Errorf("ParseWith(%q, WithStrictMode(true)) err = %v, want error: %v", tt.input, err, tt.want)
		}
	}
}

// TestReusedLayoutRefusesAnotherMonth covers the instruction that used to
// decide a field without reading the input. OpMonthName took the month from the
// instruction, resolved when the format was detected, so a reused layout
// answered with the month it was built from:
//
//	Parse("March 15, 2024").Layout.Parse("April 20, 2024") = 2024-03-20, nil
//
// Whether that was caught depended on the width of the name. "December" shifted
// the fields after it and failed the day parse, so it fell back to detection and
// came out right; "April", as wide as "March", did not.
func TestReusedLayoutRefusesAnotherMonth(t *testing.T) {
	march, err := Parse("March 15, 2024")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"April 20, 2024", "December 01, 2024", "August 09, 2024"} {
		if got, err := march.Layout.Parse(in); err == nil {
			t.Errorf("March layout accepted %q and returned %v", in, got)
		}
	}
	// It must still accept its own month, in any casing.
	for _, in := range []string{"March 15, 2024", "MARCH 15, 2024", "march 15, 2024"} {
		if _, err := march.Layout.Parse(in); err != nil {
			t.Errorf("March layout refused %q: %v", in, err)
		}
	}

	// A column of mixed months re-detects per row and agrees with detection.
	p := NewParser()
	col := []string{"March 15, 2024", "April 20, 2024", "May 03, 2024", "December 01, 2024", "June 10, 2024"}
	times, errs := p.ParseColumn(col)
	for i, in := range col {
		if errs[i] != nil {
			t.Fatalf("row %d %q: %v", i, in, errs[i])
		}
		fresh, err := Parse(in)
		if err != nil {
			t.Fatalf("row %d %q: %v", i, in, err)
		}
		if !times[i].Equal(fresh.Time) {
			t.Errorf("row %d %q: Parser gave %v, detection gives %v", i, in, times[i], fresh.Time)
		}
	}
}

// TestReusedLayoutRefusesAMonthNameInsideAWord is C24.
//
// Verifying the month name was not enough on its own. detectTextualMonth finds
// one as a whole word, so "MArAA1MAY" holds exactly one month name and it is
// MAY at offset 6, but the executor checked the three bytes the instruction
// named and nothing either side of them. A MONTH_DAY layout detected from
// "MAr A1AAA" read that input as March where detection reads it as May, and
// neither call set Ambiguous, so a caller reusing a layout down a column got a
// different month from parsing the same row on its own with nothing marking the
// difference. FuzzLayoutReuse found it and carries it as a seed.
func TestReusedLayoutRefusesAMonthNameInsideAWord(t *testing.T) {
	cached, err := Parse("MAr A1AAA")
	if err != nil {
		t.Fatal(err)
	}
	const other = "MArAA1MAY"

	fresh, err := Parse(other)
	if err != nil {
		t.Fatalf("Parse(%q): %v", other, err)
	}
	if got := fresh.Time.Month(); got != time.May {
		t.Fatalf("detection of %q reads month %v, want May; this test's premise moved", other, got)
	}

	// Refusing is the outcome. The reused layout may not answer March, and it
	// may not answer May either: the bytes it would have to read to say May are
	// not where its instructions point.
	if got, err := cached.Layout.Parse(other); err == nil {
		t.Errorf("layout detected from %q accepted %q and returned %v, want an error",
			"MAr A1AAA", other, got)
	}

	// Its own input still parses, which is what makes this a boundary rule and
	// not a ban on the shape.
	if _, err := cached.Layout.Parse("MAr A1AAA"); err != nil {
		t.Errorf("layout refused the input it was detected from: %v", err)
	}
}

// TestMonthNameNextToADigitStillParses is the case the C24 boundary must not
// break. A digit is not a word character, so a month name butted against one is
// still a whole word, and detection accepts these for exactly that reason.
// "MAY10" is in FuzzLayoutReuse's seed list from an earlier defect.
func TestMonthNameNextToADigitStillParses(t *testing.T) {
	for _, in := range []string{"MAY10", "MAY 15", "15MAY", "March15, 2024"} {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		again, err := r.Layout.Parse(in)
		if err != nil {
			t.Errorf("layout detected from %q refused it: %v", in, err)
			continue
		}
		if !again.Equal(r.Time) {
			t.Errorf("%q: reuse gave %v, detection gave %v", in, again, r.Time)
		}
	}
}

// TestMonthNameVerificationKeepsLocales guards the other direction: the check
// has to accept every spelling detection matches, or a layout refuses the very
// input it was detected from. English "sept" and a locale abbreviation with its
// trailing dot dropped are both such spellings.
func TestMonthNameVerificationKeepsLocales(t *testing.T) {
	tests := []struct {
		input string
		opts  []Option
	}{
		{"sept. 1, 2020", nil},
		{"15 mars 2024", []Option{WithLocales(FR)}},
		{"15 janv 2024", []Option{WithLocales(FR)}},
		{"15 März 2024", []Option{WithLocales(DE)}},
		{"15 marzo 2024", []Option{WithLocales(ES)}},
	}
	for _, tt := range tests {
		r, err := ParseWith(tt.input, tt.opts...)
		if err != nil {
			t.Fatalf("ParseWith(%q): %v", tt.input, err)
		}
		again, err := r.Layout.Parse(tt.input)
		if err != nil {
			t.Errorf("layout from %q refuses the input it was detected from: %v", tt.input, err)
			continue
		}
		if !again.Equal(r.Time) {
			t.Errorf("%q: reuse gave %v, detection gave %v", tt.input, again, r.Time)
		}
	}
}

// TestUTCMarkerIsRead covers the second instruction that decided without
// looking: OpTZZ set UTC on trust, so a reused layout read
// "2024-03-15T10:30:00+", a truncated offset, as UTC.
func TestUTCMarkerIsRead(t *testing.T) {
	z, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"2024-03-15T10:30:00+", "2024-03-15T10:30:00Q", "2024-03-15T10:30:00z"} {
		if got, err := z.Layout.Parse(in); err == nil {
			t.Errorf("layout accepted %q as UTC and returned %v", in, got)
		}
	}
	if _, err := z.Layout.Parse("2024-03-15T10:30:00Z"); err != nil {
		t.Errorf("layout refused its own input: %v", err)
	}
}

// TestNumericPartWiderThanItsFieldIsRefused covers a wrong answer that needed
// no reuse at all. buildDatePartFields chose the field kind by testing only for
// width one, so a three-character part got an FDay2 and a Len of 3. OpDay2
// reads exactly two, so detection validated one value and the program returned
// another:
//
//	Parse("020/01/2024") = 2024-01-02   detection resolved day 20, the op read "02"
//	Parse("3/015/2024")  = 2024-03-01   detection resolved day 15, the op read "01"
//
// Neither fuzz target could see it. FuzzParse compares Parse against
// Layout.Parse, and both run the same program, so both were wrong identically
// and agreed. Only FuzzLayoutReuse caught it, and only because two
// differently-shaped inputs put the fields in different places.
func TestNumericPartWiderThanItsFieldIsRefused(t *testing.T) {
	for _, in := range []string{
		"020/01/2024", // three-digit day
		"3/015/2024",  // three-digit day in the second position
		"020/1/0000",  //
		"013/1/2",     //
		"1/2/003",     // three-digit year, FYear4 would read four
		"1/1/1",       // one-digit year, FYear2 would read two
	} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want an error", in, got.Time)
		}
	}

	// The ordinary widths must all survive.
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"3/15/2024", "2024-03-15"},
		{"03/15/2024", "2024-03-15"},
		{"15.03.2024", "2024-03-15"},
		{"01/02/2024", "2024-01-02"},
		{"12/12/12", "2012-12-12"},
		{"1/2/24", "2024-01-02"},
		{"25/12/2024", "2024-12-25"},
	} {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if s := got.Time.UTC().Format("2006-01-02"); s != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.in, s, tt.want)
		}
	}
}

// TestSkippedRunWithADigitIsRefused covers the one thing exact byte coverage
// does not settle: which bytes a skip is allowed to cover.
//
// A skip fixes a run's width and reads nothing, on the reasoning that the
// detector scanned past those bytes because they held no value. What told the
// detector they held no value is that they were not digits, and that half was
// never carried into the program. Parse("MAY A1") builds a month name, a skip
// of width 2 over " A", and a 1-or-2 digit day:
//
//	"MAY 15"  the skip takes " 1" and the day reads "5"      → May 5th
//	"MAY1010" the skip takes "10" and the day widens to "10" → May 10th
//
// Detection reads those as May 15th and as the year 1010. The second case is
// why counting bytes cannot catch this on its own: the skip takes two bytes it
// was not given, the day takes one more than it declared, and 3 + 2 + 2 is
// still the length of the input.
func TestSkippedRunWithADigitIsRefused(t *testing.T) {
	cached, err := Parse("MAY A1")
	if err != nil {
		t.Fatalf("Parse(%q): %v", "MAY A1", err)
	}

	for _, in := range []string{
		"MAY 15",  // digit inside the skip, day stays one byte
		"MAY1010", // digit inside the skip, day widens to two
		"MAY101",  // digit inside the skip, detection refuses this one outright
	} {
		if got, err := cached.Layout.Parse(in); err == nil {
			t.Errorf("Layout(%v).Parse(%q) = %v, want an error", cached.Layout, in, got)
		}
	}

	// A run of the same width that still holds no digit is the case the skip
	// exists to serve, and it has to keep working.
	got, err := cached.Layout.Parse("MAY B2")
	if err != nil {
		t.Fatalf("Layout(%v).Parse(%q): %v", cached.Layout, "MAY B2", err)
	}
	if d := got.Day(); d != 2 {
		t.Errorf("Layout(%v).Parse(%q) day = %d, want 2", cached.Layout, "MAY B2", d)
	}

	// The weekday name is the skip that carries its weight, and reuse across
	// two different names is what a column of RFC 1123 dates needs.
	rfc, err := Parse("Fri Jul 03 2015 18:04:07 GMT+0100")
	if err != nil {
		t.Fatalf("Parse of the RFC 1123 sample: %v", err)
	}
	if _, err := rfc.Layout.Parse("Mon Jul 06 2015 18:04:07 GMT+0100"); err != nil {
		t.Errorf("layout refused a different weekday of the same width: %v", err)
	}
}

// TestSeparatorClassSeparatesTwoFormatsOfTheSameShape covers the last thing a
// literal was not checking: which character class its byte belongs to.
//
// Refusing a digit is not enough. A bare numeric date and a time of day are
// both DD?DD?DD, and ':' is no more a digit than '-' is, so each format's
// layout accepted the other's input and answered with the wrong kind of value:
//
//	NUMERIC_MDY from "20-1-00" read "10:01:00" as 2000-01-10
//	TIME_HMS    from "10:30:45" read "12/25/24" as 12:25:24
//
// Not a day out. A date where the input held a time, and a time where it held a
// date, with no error either way. The class each position matched on was in the
// trie signature all along; the literal carries it now.
func TestSeparatorClassSeparatesTwoFormatsOfTheSameShape(t *testing.T) {
	for _, tt := range []struct{ detectFrom, applyTo string }{
		{"20-1-00", "10:01:00"},      // the crasher FuzzLayoutReuse minimised
		{"12/25/24", "10:30:00"},     // a date layout against a time
		{"3/15/24", "10:01:00"},      // the same, one-digit month
		{"10:30:45", "12/25/24"},     // a time layout against a date
		{"00:00:00", "12/25/24"},     // the same, from an all-zero time
		{"10:30", "12/25"},           // two parts rather than three
		{"2014:03:31", "2024-03-15"}, // EXIF colons against ISO dashes
		{"2024-03-15", "2014:03:31"}, // and back the other way
	} {
		cached, err := Parse(tt.detectFrom)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.detectFrom, err)
		}
		if got, err := cached.Layout.Parse(tt.applyTo); err == nil {
			t.Errorf("layout %v from %q accepted %q and returned %v",
				cached.Layout, tt.detectFrom, tt.applyTo, got)
		}
	}

	// The acceptance the loose check existed to protect. One trie entry serves
	// every byte in a class, so a layout detected from one separator has to
	// keep reading the others: naming '-' would refuse two inputs detection
	// accepts. Narrowing to the class must not narrow past it.
	iso, err := Parse("2024-03-15")
	if err != nil {
		t.Fatalf("Parse of the ISO sample: %v", err)
	}
	for _, in := range []string{"2024-03-15", "2024/03/15", "2024.03.15"} {
		got, err := iso.Layout.Parse(in)
		if err != nil {
			t.Errorf("layout %v refused %q: %v", iso.Layout, in, err)
			continue
		}
		if s := got.UTC().Format("2006-01-02"); s != "2024-03-15" {
			t.Errorf("layout %v read %q as %s, want 2024-03-15", iso.Layout, in, s)
		}
	}
}

// TestNoCrossFormatDisagreement is the deterministic half of FuzzLayoutReuse:
// every ordered pair of the formats this library advertises, checked for the
// property the fuzzer looks for at random. A layout detected from one input,
// applied to another, either refuses or agrees with what detection says the
// second input means.
//
// It is here because the fuzzer took a corpus grown across weeks to reach the
// pair that broke, and a sweep of the documented formats reaches all of them in
// six milliseconds on every go test. It turned up no defect the fuzzer had not,
// only more of the one it found: eight disagreeing pairs where the fuzzer had
// minimised one. What the sweep does is stop them coming back.
//
// The corpus is coverageCases, so a format added there is swept against every
// other one without anybody listing it twice.
func TestNoCrossFormatDisagreement(t *testing.T) {
	corpus := make([]string, 0, len(coverageCases))
	for _, c := range coverageCases {
		corpus = append(corpus, c.input)
	}
	// The same-shape inputs the disagreements have come from, which the
	// coverage table has no reason to list: a bare numeric date against a time
	// of day, a compact date against a run of digits, EXIF colons against ISO
	// dashes.
	corpus = append(corpus,
		"20-1-00", "10:01:00", "1-2-00", "12/25/24", "25.12.24",
		"00:00:00", "00000101", "0000-001", "1030", "103045", "171113",
		"171113 14:14:20", "2014:03:31", "2014:04:08 22:05",
		"2014:04:08 22:05:13", "2014-04", "2014",
		"2014-12-16 06:20:00 UTC", "2014-04-26 05:24:37 PM",
	)

	// Detect once per input rather than once per pair. An ambiguous parse is
	// excluded for the reason FuzzLayoutReuse excludes it: where detection says
	// it had to guess, a layout that made the other choice is the other
	// reading, and the caller was told on both calls.
	type detected struct {
		in     string
		layout *Layout
		want   time.Time
	}
	var known []detected
	for _, in := range corpus {
		r, err := Parse(in)
		if err != nil || !reusable(r.Layout) || r.Ambiguous {
			continue
		}
		known = append(known, detected{in, r.Layout, r.Time})
	}

	for _, a := range known {
		for _, b := range known {
			if a.in == b.in {
				continue
			}
			// Refusing is always allowed: Parser falls back to detection.
			got, err := a.layout.Parse(b.in)
			if err != nil || got.Equal(b.want) {
				continue
			}
			t.Errorf("layout %v from %q accepted %q and disagreed with detection:\n"+
				"  reused %-14s = %v\n  fresh  %-14s = %v",
				a.layout, a.in, b.in, a.layout.String(), got, b.layout.String(), b.want)
		}
	}
}

// TestCompileHonoursTheTokensItAccepts covers W5 and W6, which are one defect
// wearing two shapes: ParseGoLayout accepted a token the rest of the pipeline
// could not honour, and said nothing.
//
// "_2" mapped to the 1-or-2 digit day op, which computes s[off]-'0' on the
// leading space, reads 251, and refuses. Only the two-digit half of what the
// token advertises was implemented, so the token could not parse the one thing
// it exists for.
//
// Z07:00 never advanced the layout parser's offset, so every field after it was
// assigned the zone's own offset. The comment said the zone is always last,
// which is true of the formats this library ships and not of a layout a caller
// writes.
func TestCompileHonoursTheTokensItAccepts(t *testing.T) {
	for _, tt := range []struct{ layout, input string }{
		// W5. The first is the case the token exists for.
		{"2006-01-_2", "2024-03- 5"},
		{"2006-01-_2", "2024-03-15"},
		{"2006-01-_2", "2024-03-05"},
		{"2006-01-_2 15:04", "2024-03- 5 10:30"},

		// W6, both zone forms and both widths.
		{"15:04:05Z07:00 2006", "10:30:00Z 2024"},
		{"15:04:05Z07:00 2006", "10:30:00+05:00 2024"},
		{"15:04:05Z07:00 2006", "10:30:00-08:00 1999"},
		{"15:04:05Z0700 2006", "10:30:00Z 2024"},
		{"15:04:05Z0700 2006", "10:30:00+0530 2024"},
		{"2006-01-02T15:04:05Z07:00 01", "2024-03-15T10:30:00Z 12"},

		// The shapes that already worked, so the fixes are additions.
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00Z"},
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00+05:00"},
		{"15:04:05Z07:00", "10:30:00Z"},
	} {
		want, err := time.Parse(tt.layout, tt.input)
		if err != nil {
			t.Fatalf("the reference itself refused %q with %q: %v", tt.input, tt.layout, err)
		}
		l, err := Compile(tt.layout)
		if err != nil {
			t.Errorf("Compile(%q): %v", tt.layout, err)
			continue
		}
		got, err := l.Parse(tt.input)
		if err != nil {
			t.Errorf("Compile(%q).Parse(%q): %v", tt.layout, tt.input, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("Compile(%q).Parse(%q) = %v, time.Parse = %v",
				tt.layout, tt.input, got, want)
		}
	}

	// The strictness in compile.go's doc comment still holds either side of
	// this: an element narrower than the layout declares is refused, where
	// time.Parse takes it.
	for _, tt := range []struct{ layout, input string }{
		{"2006-01-_2", "2024-03-5"},  // one byte where the token declares two
		{"2006-01-_2", "2024-03-  "}, // a space where the digit goes
		{"2006-01-_2", "2024-03- 0"}, // day zero
	} {
		l, err := Compile(tt.layout)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.layout, err)
		}
		if got, err := l.Parse(tt.input); err == nil {
			t.Errorf("Compile(%q).Parse(%q) = %v, want an error", tt.layout, tt.input, got)
		}
	}
}

// TestNaturalLanguageReadsTheWholeInput covers C14. Every eval* pattern matched
// a prefix of the token stream and returned without looking at the rest, so an
// input naming an absolute date came back as a time derived from the clock:
//
//	Parse("3 days ago 2024-01-01")  was three days before now
//
// This is the rule c4851ae gave the instruction executor, reaching the one path
// that never had it.
func TestNaturalLanguageReadsTheWholeInput(t *testing.T) {
	base := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	opts := []Option{WithBaseTime(base)}

	for _, tt := range []struct{ in, why string }{
		{"3 days ago 2024-01-01", "an ISO date the pattern never read"},
		{"yesterday 2024", "a year"},
		{"next friday 1999", "a year"},
		{"tomorrow at 5pm zzzz", "trailing bytes"},
		{"yesterday at", "an expression cut off after at"},
		{"3 days ago and then some", "a trailing clause"},
	} {
		if got, err := ParseWith(tt.in, opts...); err == nil {
			t.Errorf("ParseWith(%q) = %v, want an error: %s",
				tt.in, got.Time.UTC().Format("2006-01-02 15:04"), tt.why)
		}
	}

	// Everything the patterns do read has to keep working, including the
	// composed ones. CLAUDE.md: evalRelWord runs before evalNAgo so that
	// "yesterday at 5pm" is one pattern and not a pattern plus trailing text.
	// Requiring full consumption is the same hazard from the other direction.
	for _, tt := range []struct{ in, want string }{
		{"yesterday", "2026-08-13 00:00"},
		{"3 days ago", "2026-08-11 12:00"},
		{"yesterday at 5pm", "2026-08-13 17:00"},
		{"yesterday at 5", "2026-08-13 05:00"},
		{"yesterday at noon", "2026-08-13 12:00"},
		{"tomorrow at midnight", "2026-08-15 00:00"},
		{"next friday at 2pm", "2026-08-21 14:00"},
		{"in 10 minutes", "2026-08-14 12:10"},
		{"beginning of month", "2026-08-01 00:00"},
		{"a few days ago", "2026-08-11 12:00"},
		{"half an hour ago", "2026-08-14 11:30"},
		{"2 weeks and 3 days ago", "2026-07-28 12:00"},
		{"1 hour and 3 minutes ago", "2026-08-14 10:57"},
		{"last monday", "2026-08-10 00:00"},
		{"march 15", "2026-03-15 00:00"},
		{"december 25th", "2026-12-25 00:00"},
		{"this morning", "2026-08-14 08:00"},
		{"last night", "2026-08-13 21:00"},
		{"next week", "2026-08-17 00:00"},
		{"end of year", "2026-12-31 23:59"},
		{"sunday", "2026-08-09 00:00"},
		{"3 days ago.", "2026-08-11 12:00"}, // the scanner drops punctuation
		{"  tomorrow  ", "2026-08-15 00:00"},
	} {
		got, err := ParseWith(tt.in, opts...)
		if err != nil {
			t.Errorf("ParseWith(%q): %v", tt.in, err)
			continue
		}
		if s := got.Time.UTC().Format("2006-01-02 15:04"); s != tt.want {
			t.Errorf("ParseWith(%q) = %s, want %s", tt.in, s, tt.want)
		}
	}
}

// TestPaddedTextualDateKeepsItsYear covers C13, which is what a fixed-width
// column meets: a date behind enough padding was never offered to the textual
// detector, because the condition was "has a letter in the first 64 bytes"
// wearing the name HasLetter. Parse fell through to natural language, which
// read "March 15" and answered with the base year.
//
// The failure had no error and no Ambiguous flag on it. The only signal a
// caller got was Kind, and a caller reading Kind on every row is a caller who
// already suspects something.
func TestPaddedTextualDateKeepsItsYear(t *testing.T) {
	for _, pad := range []int{0, 1, 63, 64, 65, 100, 200} {
		in := strings.Repeat(" ", pad) + "March 15, 2024"
		got, err := Parse(in)
		if err != nil {
			t.Errorf("Parse with %d bytes of padding: %v", pad, err)
			continue
		}
		if s := got.Time.UTC().Format("2006-01-02"); s != "2024-03-15" {
			t.Errorf("Parse with %d bytes of padding = %s, want 2024-03-15 (layout %v, kind %v)",
				pad, s, got.Layout, got.Kind)
		}
		if got.Kind != KindAbsolute {
			t.Errorf("Parse with %d bytes of padding: Kind = %v, want absolute", pad, got.Kind)
		}
	}

	// Other shapes in the same family, which were refused outright rather than
	// answered wrongly, and should now parse.
	for _, tt := range []struct{ in, want string }{
		{strings.Repeat(" ", 70) + "15 Mar 2024", "2024-03-15"},
		{strings.Repeat(" ", 70) + "Fri, 15 Mar 2024 10:30:00 +0000", "2024-03-15"},
		{strings.Repeat("\t", 70) + "March 15, 2024", "2024-03-15"},
	} {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q...): %v", tt.in[:6], err)
			continue
		}
		if s := got.Time.UTC().Format("2006-01-02"); s != tt.want {
			t.Errorf("Parse(...%q) = %s, want %s", tt.in[len(tt.in)-14:], s, tt.want)
		}
	}

	// Past what a program can address, the C4 guard takes over. Refusing is the
	// right answer there and this pins that it is a refusal and not a wrong
	// offset.
	//
	// The filler is hyphens rather than the spaces this used to use. F9 trims
	// surrounding whitespace before anything looks at the input, so padding no
	// longer pushes a field anywhere: 300 spaces and one space describe the
	// same value and both parse. What still reaches the guard is a run of
	// something that is not padding, which is what a field past byte 255
	// actually needs.
	if got, err := Parse(strings.Repeat("-", 300) + "March 15, 2024"); err == nil {
		t.Errorf("Parse with 300 bytes of leading filler = %v, want an error", got.Time)
	}

	// And the case that moved: whitespace of any length is padding, so the
	// value is the same value and the length bound applies to it rather than to
	// what surrounds it. 614 bytes is past compile.MaxDescribableLen, which
	// used to refuse this at the door.
	if got, err := Parse(strings.Repeat(" ", 600) + "March 15, 2024"); err != nil {
		t.Errorf("Parse with 600 bytes of padding: %v, want 2024-03-15", err)
	} else if s := got.Time.UTC().Format("2006-01-02"); s != "2024-03-15" {
		t.Errorf("Parse with 600 bytes of padding = %s, want 2024-03-15", s)
	}

	// A genuinely relative string is still natural language. The fix moves
	// inputs from NL to structured detection, and it must not move these.
	rel, err := Parse("3 days ago")
	if err != nil {
		t.Fatalf(`Parse("3 days ago"): %v`, err)
	}
	if rel.Kind != KindRelative {
		t.Errorf(`Parse("3 days ago") Kind = %v, want relative`, rel.Kind)
	}
}

// TestCompileRefusesALayoutItCannotRepresent covers C4, which was a wrong time
// when it was filed and had already decayed into a confusing error by the time
// it was worked.
//
// Compile stopped filling the program at MaxInstructions and returned what it
// had, with a nil error. ParseGoLayout emits one instruction per unrecognised
// layout byte, so a long enough run of literal text ran the count out before
// the first field and the layout answered year zero for every input. c4851ae
// turned that into an error at parse time, which blamed the input: "layout
// describes 24 of 37 bytes" is a true sentence about the wrong thing.
func TestCompileRefusesALayoutItCannotRepresent(t *testing.T) {
	// Layouts inside the budget keep working and keep agreeing with the stdlib.
	for _, tt := range []struct{ layout, input string }{
		{"2006-01-02", "2024-03-15"},
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00Z"},
		{"Date: 2006-01-02", "Date: 2024-03-15"},
	} {
		l, err := Compile(tt.layout)
		if err != nil {
			t.Errorf("Compile(%q): %v", tt.layout, err)
			continue
		}
		got, err := l.Parse(tt.input)
		if err != nil {
			t.Errorf("Compile(%q).Parse(%q): %v", tt.layout, tt.input, err)
			continue
		}
		want, err := time.Parse(tt.layout, tt.input)
		if err != nil {
			t.Fatalf("the reference itself refused %q: %v", tt.input, err)
		}
		if !got.Equal(want) {
			t.Errorf("Compile(%q).Parse(%q) = %v, time.Parse = %v",
				tt.layout, tt.input, got, want)
		}
	}

	// Past the budget it has to be the constructor that refuses, not the parse.
	// One instruction per unrecognised layout byte is what spends it, so these
	// are layouts the stdlib handles and this library will not. That is the
	// trade: a refusal a caller sees at construction beats a Layout that fails
	// on every value.
	for _, layout := range []string{
		strings.Repeat("x", 70) + "2006-01-02",
		"The current date and time: 2006-01-02", // 32 instructions
		"Generated on 2006-01-02 at 15:04",      // 25
	} {
		if l, err := Compile(layout); err == nil {
			t.Errorf("Compile(%q) returned a Layout for %d instructions, limit is %d",
				layout, len(l.goLayout), 24)
		}
	}

	// MustCompile is the same constructor, so it has to panic rather than hand
	// back a layout that cannot work.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("MustCompile of an over-long layout did not panic")
			}
		}()
		MustCompile(strings.Repeat("x", 70) + "2006-01-02")
	}()
}

// TestEpochOverflowAndSign covers what a caller sees for the two epoch bugs,
// which were found by reading rather than by any test: internal/epoch had no
// fuzz target, and it is the one package doing unchecked integer arithmetic on
// untrusted input.
func TestEpochOverflowAndSign(t *testing.T) {
	// Nineteen digits reach past int64 and used to wrap into a confident
	// wrong date, the third of these into the future despite its minus.
	for _, in := range []string{
		"9999999999999999999",  // was 1702-05-02
		"9223372036854775808",  // was 1677-09-21
		"-9999999999999999999", // was 2237-09-01
		"-9223372036854775808", // was 1677-09-21
	} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want an error", in, got.Time)
		}
	}

	for _, tt := range []struct {
		in   string
		want time.Time
	}{
		{"9223372036854775807", time.Unix(0, 9223372036854775807)},
		{"-1710500000123", time.UnixMilli(-1710500000123)},
		{"-1710500000123456", time.UnixMicro(-1710500000123456)},
		{"-1710500000123456789", time.Unix(0, -1710500000123456789)},
		{"-1710500000.5", time.Unix(-1710500000, -500000000)},
		{"1710500000.99999999999999999999999", time.Unix(1710500000, 999999999)},
	} {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if !got.Time.Equal(tt.want) {
			t.Errorf("Parse(%q) = %v, want %v (off by %v)",
				tt.in, got.Time.UTC(), tt.want.UTC(), got.Time.Sub(tt.want))
		}
	}
}

// TestLiteralHoldingADigitIsRefused is the same rule as
// TestSkippedRunWithADigitIsRefused reaching the other instruction that does
// not read what it covers.
//
// A trie format cannot name the byte at a literal, because it matches on a
// signature of character classes and one entry serves the whole class:
// ISO8601_DATE reads "2024-03-15", "2024/03/15" and "2024.03.15". No class at a
// literal position holds a digit, though, and a digit is what changes which
// format an input is.
func TestLiteralHoldingADigitIsRefused(t *testing.T) {
	for _, tt := range []struct {
		detectFrom string
		applyTo    string
		why        string
	}{
		// Read one second past midnight where detection reads the first of
		// January in the year 0.
		{"00:00:00", "00000101", "TIME_HMS against a compact date"},
		{"10:30", "1030", "TIME_HM against a bare year"},
		{"10:30", "10300", "TIME_HM against digits detection refuses"},
		{"2024-03-15", "20240315", "ISO8601_DATE against a compact date"},
	} {
		if got, err := Parse(tt.detectFrom); err == nil {
			if ts, err := got.Layout.Parse(tt.applyTo); err == nil {
				t.Errorf("%s: Layout(%v).Parse(%q) = %v, want an error",
					tt.why, got.Layout, tt.applyTo, ts)
			}
		} else {
			t.Fatalf("Parse(%q): %v", tt.detectFrom, err)
		}
	}

	// The separator variants a signature class admits all still reuse, which is
	// why the byte itself cannot be the check.
	iso, err := Parse("2024-03-15")
	if err != nil {
		t.Fatalf("Parse of the ISO sample: %v", err)
	}
	for _, in := range []string{"2024-03-15", "2024/03/15", "2024.03.15"} {
		got, err := iso.Layout.Parse(in)
		if err != nil {
			t.Errorf("Layout(%v).Parse(%q): %v", iso.Layout, in, err)
			continue
		}
		if s := got.UTC().Format("2006-01-02"); s != "2024-03-15" {
			t.Errorf("Layout(%v).Parse(%q) = %s, want 2024-03-15", iso.Layout, in, s)
		}
	}
}

// TestImpossibleDayIsRefused is C23, and the table is M10's third piece: a
// corpus generator cannot produce these by construction, so they are written
// out.
//
// The day was range-checked against the constant 31 and never against its
// month, so "2024-02-30" came back as the first of March, with a nil error, on
// every date-bearing format. time.Parse refuses all of it by name. The two
// checks in the same function, for the ISO week and the ordinal day, are the
// same defect fixed twice already; this is the third.
func TestImpossibleDayIsRefused(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	refused := []struct{ in, why string }{
		// February, which is where it matters most.
		{"2024-02-30", "February has 29 days in 2024"},
		{"2024-02-31", ""},
		{"2023-02-29", "2023 is not a leap year"},
		{"1900-02-29", "1900 is not a leap year, the century rule"},
		{"2100-02-29", "nor is 2100"},

		// The thirty-day months.
		{"2024-04-31", "April has 30"},
		{"2024-06-31", "June has 30"},
		{"2024-09-31", "September has 30"},
		{"2024-11-31", "November has 30"},

		// The same date through every format family that carries one, because
		// the check has to be in the executor and not in one detector.
		{"2024-02-30T10:30:00Z", "ISO with a zone"},
		{"2024-02-30T10:30:00+05:30", "RFC 3339"},
		{"2024-02-30 10:30:00", "SQL"},
		{"2024-02-30 10:30:00.123", "SQL with a fraction"},
		{"20240230", "compact"},
		{"20240230T103000", "compact with a time"},
		{"02/30/2024", "US numeric"},
		{"30.02.2024", "European numeric"},
		{"2024/02/30", "Asian numeric"},
		{"February 30, 2024", "textual month first"},
		{"30 February 2024", "textual day first"},
		{"Feb 30, 2024", "abbreviated"},
		{"30-Feb-2024", "spreadsheet"},
		{"Fri, 30 Feb 2024 10:30:00 +0000", "RFC 2822"},
		{"30/Feb/2024:10:30:00 +0000", "common log format"},
		{"2/30/2024 10:30:00 AM", "spreadsheet with a time"},
		{"February 30", "no year, so it is the base year that decides"},
	}
	for _, c := range refused {
		if _, err := ParseWith(c.in, WithBaseTime(base)); err == nil {
			t.Errorf("ParseWith(%q) returned no error; that day does not exist (%s)", c.in, c.why)
		}
	}

	// The boundary from the other side. A check that refuses too much is the
	// same defect facing the other way, and February is where an off-by-one
	// would land.
	accepted := []struct{ in, want string }{
		{"2024-02-29", "2024-02-29"}, // a leap year
		{"2000-02-29", "2000-02-29"}, // and the century exception to the century rule
		{"2023-02-28", "2023-02-28"},
		{"2024-01-31", "2024-01-31"},
		{"2024-04-30", "2024-04-30"},
		{"2024-12-31", "2024-12-31"},
		{"2024-02-30", ""}, // sentinel, replaced below
	}
	accepted = accepted[:len(accepted)-1]
	for _, c := range accepted {
		r, err := ParseWith(c.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.in, err)
			continue
		}
		if got := r.Time.Format("2006-01-02"); got != c.want {
			t.Errorf("ParseWith(%q) = %s, want %s", c.in, got, c.want)
		}
	}

	// A compiled layout has to refuse them too, not only the detecting call:
	// the layout is what a caller keeps for the rest of the column.
	lay, err := Detect("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"2024-02-30", "2023-02-29", "2024-04-31"} {
		if _, err := lay.Parse(in); err == nil {
			t.Errorf("Layout.Parse(%q) returned no error", in)
		}
	}
	if _, err := lay.Parse("2024-02-29"); err != nil {
		t.Errorf("Layout.Parse(\"2024-02-29\"): %v", err)
	}
}

// TestLeapSecondIsRefused is C24, found by the over-acceptance half of the
// oracle on its first two-minute run.
//
// The second was range-checked at 0 to 60 with "60 for leap second" beside it.
// Accepting it was deliberate and what it answered was not: 60 fell through to
// time.Date, which normalised it, so a leap second moved the date. It moved on
// exactly the two dates of the year one can be announced.
func TestLeapSecondIsRefused(t *testing.T) {
	for _, in := range []string{
		"2016-12-31T23:59:60Z",  // a real leap second, the last one announced
		"2024-06-30 23:59:60",   // the other date one can fall on
		"20240630235960",        // compact, so it is not the colon being checked
		"2024-03-15 10:30:60",   // and any other second, which was never a leap one
		"2024-03-15T10:30:60Z",  //
		"3/15/2024 10:30:60 AM", //
	} {
		if _, err := ParseWith(in, WithBaseTime(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))); err == nil {
			t.Errorf("ParseWith(%q) returned no error; a second of 60 rolled the date forward", in)
		}
	}
	// 59 still parses, on both executor paths.
	for _, in := range []string{"2016-12-31T23:59:59Z", "20161231235959"} {
		if _, err := Parse(in); err != nil {
			t.Errorf("Parse(%q): %v", in, err)
		}
	}
}

// TestGoLayoutDescribesTheInputItCameFrom is W18.
//
// A signature is a sequence of character classes, so one trie entry matches
// every spelling of its separators: "2024-03-15", "2024/03/15" and "2024.03.15"
// are one entry, which is deliberate and is why all three parse. The entry
// carries one Go layout, and GoLayout used to hand back the dashed spelling for
// all three. A caller who took it and gave it to time.Parse for the next row of
// their column got an error per row.
//
// The property is the one the accessor's name promises: the layout it returns
// parses the input it was detected from.
func TestGoLayoutDescribesTheInputItCameFrom(t *testing.T) {
	inputs := []string{
		// The canonical spellings.
		"2024-03-15",
		"2024-03-15T10:30:00",
		"2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00+05:30",
		"2024-03-15 10:30:00",
		"2024-03-15 10:30:00.123",
		"2024-03-15 10:30:00+05:30",
		"10:30", "10:30:00", "10:30:00.123", "10:30 PM",
		"20240315",

		// The same formats, spelled the other ways the class admits. Each one
		// used to report the dashed or dotted layout of its entry.
		"2024/03/15",
		"2024.03.15",
		"2024/03/15 10:30:00",
		"2024.03.15 10:30:00",
		"2024/03/15T10:30:00Z",

		// A fraction spelled with anything but a dot is the one case that gets
		// no Go layout rather than a respelled one: ".000" is a token, and
		// writing another byte over its dot leaves a layout that reads
		// "00:00:00/000" and refuses "00:00:00/010".
		"10:30:00/000",
		"10:30:00/123",
	}
	for _, in := range inputs {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		gl, ok := r.Layout.GoLayout()
		if !ok {
			continue // no stdlib equivalent claimed, so nothing to check
		}
		got, perr := time.Parse(gl, in)
		if perr != nil {
			t.Errorf("Parse(%q).Layout.GoLayout() = %q, which time.Parse refuses for that "+
				"same input: %v", in, gl, perr)
			continue
		}
		// A format with no year in it takes the base year here and year zero in
		// the stdlib, which is W3's decision and not this card's business, so
		// the comparison drops the year when the stdlib had none. The oracle
		// does the same, for the same reason.
		want := r.Time
		if got.Year() == 0 {
			got, want = flattenYear(got), flattenYear(want)
		}
		if !got.Equal(want) {
			t.Errorf("Parse(%q) = %v but its own GoLayout %q reads %v", in, r.Time, gl, got)
		}
	}
}

// TestSeparatorClassIsDeliberatelyWide pins what the character classes accept,
// which is more than any producer writes and is not an accident.
//
// A trie entry is keyed by a signature, and a signature is a sequence of
// character classes rather than of bytes. That is what makes "2024/03/15" and
// "2024.03.15" parse from the same entry as "2024-03-15", which is the point:
// naming "-" in the entry would refuse the other two, and real systems emit
// them. The cost is that a class holds bytes no format uses, so
// "0000.01.01+00:00:00" is a datetime here.
//
// Narrowing it means splitting the class, which changes every signature holding
// the byte and rekeys the trie. Nothing is wrong with the instants, so this is
// written down rather than changed. If it is ever narrowed, this test is the
// list of what stops parsing.
func TestSeparatorClassIsDeliberatelyWide(t *testing.T) {
	accepted := []struct{ in, want string }{
		// The separator class is - / . at every position that holds one.
		{"2024-03-15", "2024-03-15"},
		{"2024/03/15", "2024-03-15"},
		{"2024.03.15", "2024-03-15"},
		{"2024-03/15", "2024-03-15"}, // and they need not agree with each other
		{"2024/03.15", "2024-03-15"},

		// The date and the time are told apart by a class that holds T, Z, -,
		// + and a comma, so every one of them parses where the ISO T belongs.
		{"2024-03-15T10:30:00", "2024-03-15"},
		{"2024-03-15+10:30:00", "2024-03-15"},
		{"2024-03-15,10:30:00", "2024-03-15"},
		{"0000.01.01+00:00:00", "0000-01-01"},

		// And the fraction's separator is the ordinary one.
		{"10:30:00.123", "0000-01-01"},
		{"10:30:00/123", "0000-01-01"},
	}
	base := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range accepted {
		r, err := ParseWith(c.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v; the class this input relies on is deliberate,\n"+
				"  so if it has been narrowed, say so here and in the commit", c.in, err)
			continue
		}
		if got := r.Time.Format("2006-01-02"); got != c.want {
			t.Errorf("ParseWith(%q) = %s, want %s", c.in, got, c.want)
		}
	}

	// What the class does not hold. A letter is not a separator and a digit is
	// not a literal, so these are refused and the class is wide rather than
	// absent.
	for _, in := range []string{"2024x03x15", "2024-03x15", "20240315T10:30:00"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) returned no error", in)
		}
	}
}

// TestGoTimeStringTailIsBounded is W16.
//
// OpTail is the one field whose width is whatever is left, because a Go time
// string ends with a zone name and an optional monotonic reading and no
// fixed-width program describes those. Whatever is left was unbounded, so
// "2024-03-15 10:30:00 +0000 UTC" followed by a megabyte of anything was a
// date, and so was the same line followed by "; DROP TABLE users". The instant
// was right and the acceptance was not: a caller using Parse to answer "is this
// field a date" got yes for a megabyte of prose with a timestamp on the front.
func TestGoTimeStringTailIsBounded(t *testing.T) {
	const stem = "2024-03-15 10:30:00 +0000 UTC"

	// What Go itself writes. Every one of these has to keep parsing.
	for _, in := range []string{
		stem,
		stem + " m=+0.000000001",
		stem + " m=+9223372036.854775807", // the monotonic clock at int64
		"2024-03-15 10:30:00.123456789 +0545 +0545",    // Kathmandu, a numeric abbreviation
		"2024-07-15 10:30:00 +0200 CEST",               //
		"2024-03-15 10:30:00 +0000 UTC m=+0.003691376", // what time.Now() prints
	} {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if got := r.Time.Format("2006-01-02T15:04:05"); got != "2024-03-15T10:30:00" &&
			got != "2024-07-15T10:30:00" {
			t.Errorf("Parse(%q) = %s", in, got)
		}
	}

	// And what nobody writes.
	for _, suffix := range []string{
		" ; DROP TABLE users",
		" and then some entirely different sentence about something else",
		strings.Repeat("x", 1<<20),
		" " + strings.Repeat("z", 64),
	} {
		if _, err := Parse(stem + suffix); err == nil {
			t.Errorf("Parse(%q + %d bytes) returned no error", stem, len(suffix))
		}
	}

	// The boundary, from both sides, so that the constant is pinned and not
	// merely large. The tail is everything after the numeric offset, so it
	// starts at the space before the zone name and a zone name of 63 bytes
	// makes a tail of 64.
	head := "2024-03-15 10:30:00 +0000"
	for _, n := range []int{1, 62, 63} {
		if _, err := Parse(head + " " + strings.Repeat("x", n)); err != nil {
			t.Errorf("Parse with a %d-byte tail: %v", n+1, err)
		}
	}
	if _, err := Parse(head + " " + strings.Repeat("x", 64)); err == nil {
		t.Error("Parse with a 65-byte tail returned no error")
	}

	// The shape, which is what makes the bound worth having. A tail is a zone
	// name and at most one more piece, and that piece is the monotonic clock.
	for _, tail := range []string{
		" UTC", " +0545", " Europe/Berlin", " UTC m=+0.003691376",
		" m=+9223372036.854775807", "",
	} {
		if _, err := Parse(head + tail); err != nil {
			t.Errorf("Parse(%q): %v; that is a tail Go writes", head+tail, err)
		}
	}
	for _, tail := range []string{
		" UTC ; DROP TABLE users", " UTC user=bob action=delete", " UTC 2025-01-01",
		" UTC and then a sentence", "UTC", " UTC  m=+1", " UTC m+1", " UTC x=1",
	} {
		if _, err := Parse(head + tail); err == nil {
			t.Errorf("Parse(%q) returned no error; nothing writes that", head+tail)
		}
	}

	// A layout has to refuse it too, not only the detecting call: the layout is
	// what a caller keeps for the rest of their column.
	lay, err := Detect(stem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lay.Parse(stem + strings.Repeat("x", 1<<10)); err == nil {
		t.Error("Layout.Parse accepted a 1 KiB tail")
	}
	if _, err := lay.Parse(stem + " m=+0.000000001"); err != nil {
		t.Errorf("Layout.Parse(monotonic): %v", err)
	}
}
