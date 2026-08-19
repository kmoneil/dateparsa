package dateparsa

import (
	"testing"
	"time"
)

// Fixed base time: 2024-03-15 12:00:00 UTC (Friday)
var nlBase = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

func TestParse_NL_RelativeWords(t *testing.T) {
	tests := []struct {
		input string
		kind  Kind
	}{
		{"now", KindNow},
		{"today", KindNow},
		{"yesterday", KindRelative},
		{"tomorrow", KindRelative},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if result.Kind != tt.kind {
				t.Errorf("Parse(%q) kind = %v, want %v", tt.input, result.Kind, tt.kind)
			}
			if result.Layout != LayoutNaturalLanguage {
				t.Errorf("NL parse should have LayoutNaturalLanguage, got %v", result.Layout)
			}
		})
	}
}

func TestParse_NL_NDaysAgo(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		// Literal dates, not nlBase.AddDate(...). The expectation used to be
		// computed with the same call the parser makes, which made this an
		// assertion that AddDate equals AddDate; it stayed green while
		// "1 month ago" from the 31st of a month returned a date in that same
		// month. See C25 and internal/natural/arith_test.go.
		{"3 days ago", time.Date(2024, 3, 12, 12, 0, 0, 0, time.UTC)},
		{"2 weeks ago", time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)},
		{"1 month ago", time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC)},
		{"5 hours ago", time.Date(2024, 3, 15, 7, 0, 0, 0, time.UTC)},
		{"a week ago", time.Date(2024, 3, 8, 12, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
			if result.Kind != KindRelative {
				t.Errorf("kind = %v, want KindRelative", result.Kind)
			}
		})
	}
}

func TestParse_NL_InN(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"in 3 days", nlBase.AddDate(0, 0, 3)},
		{"in 10 minutes", nlBase.Add(10 * time.Minute)},
		{"in 1 week", nlBase.AddDate(0, 0, 7)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_NL_FromNow(t *testing.T) {
	result, err := ParseWith("3 days from now", WithBaseTime(nlBase))
	if err != nil {
		t.Fatal(err)
	}
	expected := nlBase.AddDate(0, 0, 3)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_NL_SelectorWeekday(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"last monday", time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
		{"next monday", time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC)},
		{"last friday", time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)},
		{"next friday", time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_NL_SelectorUnit(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"last week", time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)},
		{"next week", time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC)},
		{"last month", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"next month", time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"last year", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"next year", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_NL_Combined(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"yesterday at 5pm", time.Date(2024, 3, 14, 17, 0, 0, 0, time.UTC)},
		{"tomorrow at noon", time.Date(2024, 3, 16, 12, 0, 0, 0, time.UTC)},
		{"tomorrow at midnight", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"next friday at 14:00", time.Date(2024, 3, 22, 14, 0, 0, 0, time.UTC)},
		{"3 days ago at 5pm", time.Date(2024, 3, 12, 17, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_NL_Boundary(t *testing.T) {
	tests := []struct {
		input string
		check func(time.Time) bool
		desc  string
	}{
		{
			"beginning of month",
			func(t time.Time) bool { return t.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) },
			"March 1",
		},
		{
			"end of year",
			func(t time.Time) bool { return t.Month() == 12 && t.Day() == 31 },
			"Dec 31",
		},
		{
			"start of day",
			func(t time.Time) bool { return t.Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)) },
			"midnight today",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !tt.check(result.Time) {
				t.Errorf("Parse(%q) = %v, expected %s", tt.input, result.Time, tt.desc)
			}
		})
	}
}

func TestParse_NL_SelectorMonth(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"last january", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"next january", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(nlBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_NL_DoesNotFalsePositive(t *testing.T) {
	// These should parse as structured dates, not NL.
	structuredInputs := []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"March 15, 2024",
		"20240315",
	}
	for _, input := range structuredInputs {
		result, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
			continue
		}
		if result.Kind != KindAbsolute {
			t.Errorf("Parse(%q) kind = %v, want KindAbsolute (structured detection should take priority)", input, result.Kind)
		}
	}
}

func TestParse_NL_CaseInsensitive(t *testing.T) {
	tests := []string{
		"Yesterday",
		"YESTERDAY",
		"3 Days Ago",
		"Next FRIDAY",
		"LAST WEEK",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseWith(input, WithBaseTime(nlBase))
			if err != nil {
				t.Errorf("Parse(%q) error: %v", input, err)
			}
		})
	}
}

// === NL Benchmarks ===

func BenchmarkParse_NL_Yesterday(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("yesterday", opts...)
	}
}

func BenchmarkParse_NL_3DaysAgo(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("3 days ago", opts...)
	}
}

func BenchmarkParse_NL_NextFriday(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("next friday", opts...)
	}
}

func BenchmarkParse_NL_YesterdayAt5pm(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("yesterday at 5pm", opts...)
	}
}

func BenchmarkParse_NL_BeginningOfMonth(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("beginning of month", opts...)
	}
}

func BenchmarkParse_NL_In10Minutes(b *testing.B) {
	opts := []Option{WithBaseTime(nlBase)}
	for b.Loop() {
		ParseWith("in 10 minutes", opts...)
	}
}

// TestParse_NL_MonthEndClamps is C25's regression at the public API. The
// arithmetic itself is covered by the property test in
// internal/natural/arith_test.go; this pins that the fix is reachable through
// Parse, through the compound form, and through the prefix-ago form the
// romance and germanic locales use, which is a fourth caller of addUnit that
// no English test touches.
func TestParse_NL_MonthEndClamps(t *testing.T) {
	// The 31st of March. Nothing below can be expressed with AddDate.
	monthEnd := time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC)
	// The 29th of February, for the year arm.
	leapDay := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input   string
		base    time.Time
		locales []Locale
		want    time.Time
	}{
		// Before the fix this was 2024-03-02: a date inside the month the
		// expression was asked to leave.
		{"1 month ago", monthEnd, nil, time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"in 1 month", monthEnd, nil, time.Date(2024, 4, 30, 12, 0, 0, 0, time.UTC)},
		{"1 month from now", monthEnd, nil, time.Date(2024, 4, 30, 12, 0, 0, 0, time.UTC)},
		{"1 year ago", leapDay, nil, time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC)},
		{"in 1 year", leapDay, nil, time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC)},

		// Months apply before days, so the 31st of March goes to the 29th of
		// February and then back one day.
		{"1 month and 1 day ago", monthEnd, nil, time.Date(2024, 2, 28, 12, 0, 0, 0, time.UTC)},

		// evalPrefixAgo, the caller with no English spelling.
		{"il y a 1 mois", monthEnd, []Locale{FR}, time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"hace 1 mes", monthEnd, []Locale{ES}, time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"vor 1 Monat", monthEnd, []Locale{DE}, time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)},

		// Shifts that need no clamp must not move.
		{"2 months ago", monthEnd, nil, time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC)},
		{"1 year ago", monthEnd, nil, time.Date(2023, 3, 31, 12, 0, 0, 0, time.UTC)},
		{"12 months ago", monthEnd, nil, time.Date(2023, 3, 31, 12, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.input+"/"+tt.base.Format("2006-01-02"), func(t *testing.T) {
			opts := []Option{WithBaseTime(tt.base)}
			if len(tt.locales) > 0 {
				opts = append(opts, WithLocales(tt.locales...))
			}
			result, err := ParseWith(tt.input, opts...)
			if err != nil {
				t.Fatalf("ParseWith(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.want) {
				t.Errorf("ParseWith(%q, base %s) = %s, want %s",
					tt.input, tt.base.Format(time.RFC3339),
					result.Time.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
			if result.Kind != KindRelative {
				t.Errorf("kind = %v, want KindRelative", result.Kind)
			}
		})
	}
}
