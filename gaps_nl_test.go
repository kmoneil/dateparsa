package dateparsa

import (
	"testing"
	"time"
)

// Base time: 2024-03-15 12:00:00 UTC (Friday)
var gapNLBase = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

func gapNLOpts(extra ...Option) []Option {
	return append([]Option{WithBaseTime(gapNLBase)}, extra...)
}

// TestGaps_NL_WrittenNumbers tests word-form numbers: "five minutes ago".
func TestGaps_NL_WrittenNumbers(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"five minutes ago", gapNLBase.Add(-5 * time.Minute)},
		{"three days ago", gapNLBase.AddDate(0, 0, -3)},
		{"two weeks from now", gapNLBase.AddDate(0, 0, 14)},
		{"one hour ago", gapNLBase.Add(-1 * time.Hour)},
		{"ten minutes ago", gapNLBase.Add(-10 * time.Minute)},
		{"in seven days", gapNLBase.AddDate(0, 0, 7)},
		{"twenty seconds ago", gapNLBase.Add(-20 * time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_FewAndHalf tests "few" and "half" quantifiers.
func TestGaps_NL_FewAndHalf(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"a few days ago", gapNLBase.AddDate(0, 0, -3)},
		{"a few hours ago", gapNLBase.Add(-3 * time.Hour)},
		{"half an hour ago", gapNLBase.Add(-30 * time.Minute)},
		{"half a day ago", gapNLBase.Add(-12 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_TimeOfDay tests "tonight", "last night", "this morning", etc.
func TestGaps_NL_TimeOfDay(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		// tonight = today at 21:00
		{"tonight", time.Date(2024, 3, 15, 21, 0, 0, 0, time.UTC)},
		// last night = yesterday at 21:00
		{"last night", time.Date(2024, 3, 14, 21, 0, 0, 0, time.UTC)},
		// this morning = today at 08:00
		{"this morning", time.Date(2024, 3, 15, 8, 0, 0, 0, time.UTC)},
		// this afternoon = today at 14:00
		{"this afternoon", time.Date(2024, 3, 15, 14, 0, 0, 0, time.UTC)},
		// this evening = today at 18:00
		{"this evening", time.Date(2024, 3, 15, 18, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_CompoundDurations tests "1 hour and 3 minutes from now".
func TestGaps_NL_CompoundDurations(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"1 hour and 3 minutes ago", gapNLBase.Add(-1*time.Hour - 3*time.Minute)},
		{"2 hours and 30 minutes from now", gapNLBase.Add(2*time.Hour + 30*time.Minute)},
		{"in 1 day and 6 hours", gapNLBase.AddDate(0, 0, 1).Add(6 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_OrdinalDates tests "december 25th", "the 5th of next month".
func TestGaps_NL_OrdinalDates(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		// "december 25th" — December 25 of current (or contextual) year
		{"december 25th", time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)},
		{"november 1st", time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"march 3rd", time.Date(2024, 3, 3, 0, 0, 0, 0, time.UTC)},
		// With time
		{"december 25th at 5pm", time.Date(2024, 12, 25, 17, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_BareWeekday tests "sunday" alone (no last/next prefix).
func TestGaps_NL_BareWeekday(t *testing.T) {
	// Base is Friday March 15, 2024.
	// Bare weekday should default to past (most recent occurrence).
	tests := []struct {
		input    string
		expected time.Time
	}{
		// Last Sunday = March 10
		{"sunday", time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)},
		// Last Monday = March 11
		{"monday", time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
		// Last Wednesday = March 13
		{"wednesday", time.Date(2024, 3, 13, 0, 0, 0, 0, time.UTC)},
		// Thursday = March 14 (yesterday)
		{"thursday", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		// Friday = today, but "friday" alone means last Friday = March 8
		{"friday", time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, gapNLOpts()...)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}

	// With PreferFuture, bare weekday means next occurrence.
	t.Run("sunday prefer future", func(t *testing.T) {
		result, err := ParseWith("sunday", WithBaseTime(gapNLBase), WithPreferFuture(true))
		if err != nil {
			t.Fatal(err)
		}
		// Next Sunday = March 17
		expected := time.Date(2024, 3, 17, 0, 0, 0, 0, time.UTC)
		if !result.Time.Equal(expected) {
			t.Errorf("got %v, want %v", result.Time, expected)
		}
	})
}

// TestGaps_NL_LocalizedRelative tests non-English "N units ago" patterns.
func TestGaps_NL_LocalizedRelative(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		locale   Locale
		expected time.Time
	}{
		// French: "il y a 2 heures"
		{
			name: "French 2 hours ago", input: "il y a 2 heures",
			locale:   FR,
			expected: gapNLBase.Add(-2 * time.Hour),
		},
		// Spanish: "hace 3 dias"
		{
			name: "Spanish 3 days ago", input: "hace 3 dias",
			locale:   ES,
			expected: gapNLBase.AddDate(0, 0, -3),
		},
		// German: "vor 5 Minuten"
		{
			name: "German 5 minutes ago", input: "vor 5 Minuten",
			locale:   DE,
			expected: gapNLBase.Add(-5 * time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(gapNLBase), WithLocales(tt.locale))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}

// TestGaps_NL_Regression ensures existing NL expressions still work.
func TestGaps_NL_Regression(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"now", gapNLBase},
		{"today", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"yesterday", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"tomorrow", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"3 days ago", gapNLBase.AddDate(0, 0, -3)},
		{"in 10 minutes", gapNLBase.Add(10 * time.Minute)},
		{"last friday", time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)},
		{"next monday", time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC)},
		{"yesterday at 5pm", time.Date(2024, 3, 14, 17, 0, 0, 0, time.UTC)},
		{"beginning of month", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"a week ago", gapNLBase.AddDate(0, 0, -7)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithBaseTime(gapNLBase))
			if err != nil {
				t.Fatalf("REGRESSION: Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("REGRESSION: got %v, want %v", result.Time, tt.expected)
			}
		})
	}
}
