package dateparsa

import (
	"errors"
	"testing"
	"time"
)

// TestParse_SkipRunWordRefused is C26 at the public boundary.
//
// The detect package has the table that covers the rule; this one is here
// because the promise the rule keeps is Parse's. Each input below used to
// return a time, a nil error and Ambiguous false, and the day was not the day
// the words name. An error is the answer now, and it has to arrive as
// ErrNoMatch so that errors.Is works on it like any other refusal.
func TestParse_SkipRunWordRefused(t *testing.T) {
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	inputs := []struct {
		input string
		was   time.Time // what it answered before, and what the words mean
		means time.Time
	}{
		{
			"first monday of march 2024",
			time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			"last day of february 2024",
			time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			"end of march 2024",
			time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range inputs {
		r, err := ParseWith(tt.input, WithBaseTime(base))
		if err == nil {
			t.Errorf("ParseWith(%q) = %v, want an error; it used to answer %v and the words mean %v",
				tt.input, r.Time, tt.was, tt.means)
			continue
		}
		if !errors.Is(err, ErrNoMatch) {
			t.Errorf("ParseWith(%q) error = %v, want it to unwrap to ErrNoMatch", tt.input, err)
		}
	}
}

// TestParse_SkipRunWordKept is the regression half. Every one of these puts a
// word inside a run no field reads, and every one is a format the library
// promises.
func TestParse_SkipRunWordKept(t *testing.T) {
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	inputs := []string{
		"the 15th of March 2024",
		"15th of March 2024",
		"March 15, 2024",
		"15 March 2024",
	}
	for _, in := range inputs {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", in, err)
			continue
		}
		if !r.Time.Equal(want) {
			t.Errorf("Parse(%q) = %v, want %v", in, r.Time, want)
		}
	}

	// The ones carrying a time, checked separately because their want differs.
	withTime := map[string]time.Time{
		"March 15, 2024 at 10:30":         time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		"Fri, 15 Mar 2024 10:30:00 +0000": time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		"15/Mar/2024:10:30:00 +0000":      time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
	}
	for in, w := range withTime {
		r, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", in, err)
			continue
		}
		if !r.Time.Equal(w) {
			t.Errorf("Parse(%q) = %v, want %v", in, r.Time, w)
		}
	}
}
