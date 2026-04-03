package dateparsa

import (
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
	_, err := ParseWith("01/02/2024", WithStrictMode())
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
