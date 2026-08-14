package dateparsa

import (
	"errors"
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

// TestParserReportsAmbiguityLikeParse covers the non-strict half: a cache hit
// used to leave ParseResult.Ambiguous at its zero value, so the same input
// reported true through Parse and false through Parser.
func TestParserReportsAmbiguityLikeParse(t *testing.T) {
	p := NewParser()
	if _, err := p.Parse("01/02/2024"); err != nil { // seed the cache
		t.Fatal(err)
	}
	for _, s := range []string{"03/04/2024", "05/06/2024", "01/02/2024"} {
		cached, err := p.Parse(s)
		if err != nil {
			t.Fatalf("Parser.Parse(%q): %v", s, err)
		}
		fresh, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if cached.Ambiguous != fresh.Ambiguous {
			t.Errorf("%q: Parser reports Ambiguous=%v, Parse reports %v",
				s, cached.Ambiguous, fresh.Ambiguous)
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
