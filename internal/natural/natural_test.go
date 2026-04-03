package natural

import (
	"testing"
	"time"
)

// Fixed base time for deterministic tests: 2024-03-15 12:00:00 UTC (Friday)
var base = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

func TestScan_Basic(t *testing.T) {
	tokens := Scan("3 days ago")
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3", len(tokens))
	}
	if tokens[0].Kind != TokNumber || tokens[0].IntVal != 3 {
		t.Errorf("token 0: got %+v", tokens[0])
	}
	if tokens[1].Kind != TokUnit || tokens[1].UnitVal != UnitDay {
		t.Errorf("token 1: got %+v", tokens[1])
	}
	if tokens[2].Kind != TokDirection || tokens[2].DirVal != DirAgo {
		t.Errorf("token 2: got %+v", tokens[2])
	}
}

func TestScan_InlineTime(t *testing.T) {
	tokens := Scan("yesterday at 5pm")
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(tokens), tokens)
	}
	if tokens[0].Kind != TokRelWord || tokens[0].Raw != "yesterday" {
		t.Errorf("token 0: %+v", tokens[0])
	}
	if tokens[1].Kind != TokAt {
		t.Errorf("token 1: %+v", tokens[1])
	}
	if tokens[2].Kind != TokTime || tokens[2].Hour != 17 || tokens[2].Min != 0 {
		t.Errorf("token 2: %+v", tokens[2])
	}
}

func TestScan_ColonTime(t *testing.T) {
	tokens := Scan("next friday at 14:00")
	// next, friday, at, 14:00
	if len(tokens) != 4 {
		t.Fatalf("got %d tokens, want 4: %+v", len(tokens), tokens)
	}
	if tokens[3].Kind != TokTime || tokens[3].Hour != 14 || tokens[3].Min != 0 {
		t.Errorf("time token: %+v", tokens[3])
	}
}

func TestScan_FromNow(t *testing.T) {
	tokens := Scan("3 days from now")
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(tokens), tokens)
	}
	if tokens[2].Kind != TokDirection || tokens[2].DirVal != DirFromNow {
		t.Errorf("token 2: %+v", tokens[2])
	}
}

func TestScan_AWeekAgo(t *testing.T) {
	tokens := Scan("a week ago")
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3: %+v", len(tokens), tokens)
	}
	if tokens[0].Kind != TokNumber || tokens[0].IntVal != 1 {
		t.Errorf("'a' should be number 1: %+v", tokens[0])
	}
}

func TestEval_Now(t *testing.T) {
	r := Eval(Scan("now"), base, false)
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindNow {
		t.Errorf("kind = %v, want KindNow", r.Kind)
	}
	if !r.Time.Equal(base) {
		t.Errorf("got %v, want %v", r.Time, base)
	}
}

func TestEval_Today(t *testing.T) {
	r := Eval(Scan("today"), base, false)
	if r == nil {
		t.Fatal("expected result")
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestEval_Yesterday(t *testing.T) {
	r := Eval(Scan("yesterday"), base, false)
	if r == nil {
		t.Fatal("expected result")
	}
	expected := time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestEval_Tomorrow(t *testing.T) {
	r := Eval(Scan("tomorrow"), base, false)
	if r == nil {
		t.Fatal("expected result")
	}
	expected := time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestEval_NDaysAgo(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"3 days ago", base.AddDate(0, 0, -3)},
		{"1 day ago", base.AddDate(0, 0, -1)},
		{"2 weeks ago", base.AddDate(0, 0, -14)},
		{"1 month ago", base.AddDate(0, -1, 0)},
		{"1 year ago", base.AddDate(-1, 0, 0)},
		{"5 hours ago", base.Add(-5 * time.Hour)},
		{"10 minutes ago", base.Add(-10 * time.Minute)},
		{"30 seconds ago", base.Add(-30 * time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_NFromNow(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"3 days from now", base.AddDate(0, 0, 3)},
		{"2 weeks from now", base.AddDate(0, 0, 14)},
		{"1 month from now", base.AddDate(0, 1, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_InN(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"in 3 days", base.AddDate(0, 0, 3)},
		{"in 10 minutes", base.Add(10 * time.Minute)},
		{"in 2 hours", base.Add(2 * time.Hour)},
		{"in 1 week", base.AddDate(0, 0, 7)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_SelectorWeekday(t *testing.T) {
	// Base: 2024-03-15 = Friday
	tests := []struct {
		input    string
		expected time.Time
	}{
		// Last Monday = 2024-03-11
		{"last monday", time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
		// Next Monday = 2024-03-18
		{"next monday", time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC)},
		// Last Friday = 2024-03-08 (not today)
		{"last friday", time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)},
		// Next Friday = 2024-03-22
		{"next friday", time.Date(2024, 3, 22, 0, 0, 0, 0, time.UTC)},
		// Next Sunday = 2024-03-17
		{"next sunday", time.Date(2024, 3, 17, 0, 0, 0, 0, time.UTC)},
		// Last Sunday = 2024-03-10
		{"last sunday", time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_SelectorUnit(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		// Last week Monday = 2024-03-04
		{"last week", time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC)},
		// Next week Monday = 2024-03-18
		{"next week", time.Date(2024, 3, 18, 0, 0, 0, 0, time.UTC)},
		// Last month = 2024-02-01
		{"last month", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		// Next month = 2024-04-01
		{"next month", time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)},
		// Last year = 2023-01-01
		{"last year", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Next year = 2025-01-01
		{"next year", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_Boundary(t *testing.T) {
	tests := []struct {
		input string
		check func(time.Time) bool
		desc  string
	}{
		{
			"beginning of day",
			func(t time.Time) bool { return t.Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)) },
			"midnight of base day",
		},
		{
			"end of day",
			func(t time.Time) bool {
				return t.Equal(time.Date(2024, 3, 15, 23, 59, 59, 999999999, time.UTC))
			},
			"23:59:59.999999999 of base day",
		},
		{
			"beginning of month",
			func(t time.Time) bool { return t.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) },
			"March 1 midnight",
		},
		{
			"end of month",
			func(t time.Time) bool { return t.Month() == 3 && t.Day() == 31 && t.Hour() == 23 },
			"March 31 23:xx",
		},
		{
			"beginning of year",
			func(t time.Time) bool { return t.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) },
			"Jan 1 midnight",
		},
		{
			"end of year",
			func(t time.Time) bool { return t.Month() == 12 && t.Day() == 31 && t.Hour() == 23 },
			"Dec 31 23:xx",
		},
		{
			"start of week",
			func(t time.Time) bool {
				// Week of March 15 (Friday) starts Monday March 11.
				return t.Equal(time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC))
			},
			"Monday of current week",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !tt.check(r.Time) {
				t.Errorf("%q = %v, expected %s", tt.input, r.Time, tt.desc)
			}
		})
	}
}

func TestEval_Combined(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{
			"yesterday at 5pm",
			time.Date(2024, 3, 14, 17, 0, 0, 0, time.UTC),
		},
		{
			"tomorrow at noon",
			time.Date(2024, 3, 16, 12, 0, 0, 0, time.UTC),
		},
		{
			"tomorrow at midnight",
			time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			"next friday at 14:00",
			time.Date(2024, 3, 22, 14, 0, 0, 0, time.UTC),
		},
		{
			"3 days ago at 5pm",
			time.Date(2024, 3, 12, 17, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}

func TestEval_NoMatch(t *testing.T) {
	inputs := []string{
		"",
		"not a date expression",
		"hello world",
		"2024-03-15", // structured date, not NL
	}
	for _, input := range inputs {
		r := Eval(Scan(input), base, false)
		if r != nil {
			t.Errorf("Eval(%q) should return nil, got %v", input, r)
		}
	}
}

func TestParse_Integration(t *testing.T) {
	cfg := Config{BaseTime: base}
	r := Parse("3 days ago", cfg)
	if r == nil {
		t.Fatal("expected result")
	}
	expected := base.AddDate(0, 0, -3)
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestParse_RejectsStructured(t *testing.T) {
	cfg := Config{BaseTime: base}
	// Pure numbers / structured-looking strings should not match.
	r := Parse("2024-03-15", cfg)
	if r != nil {
		t.Errorf("expected nil for structured date, got %v", r)
	}
}

func TestEval_AWeekAgo(t *testing.T) {
	r := Eval(Scan("a week ago"), base, false)
	if r == nil {
		t.Fatal("expected result")
	}
	expected := base.AddDate(0, 0, -7)
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestEval_SelectorMonth(t *testing.T) {
	// Base = March 15, 2024
	tests := []struct {
		input    string
		expected time.Time
	}{
		// Last January = 2024-01-01 (same year, before current month)
		{"last january", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Next January = 2025-01-01
		{"next january", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		// Last June = 2023-06-01 (June is after March, so last June = previous year)
		{"last june", time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)},
		// Next June = 2024-06-01
		{"next june", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			r := Eval(Scan(tt.input), base, false)
			if r == nil {
				t.Fatalf("expected result for %q", tt.input)
			}
			if !r.Time.Equal(tt.expected) {
				t.Errorf("got %v, want %v", r.Time, tt.expected)
			}
		})
	}
}
