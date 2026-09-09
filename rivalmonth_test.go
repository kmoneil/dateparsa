package dateparsa

import (
	"errors"
	"testing"
	"time"
)

// TestParse_SecondMonthNameRefused is C32 at the public boundary.
//
// The detect package has the table that covers the rule; this one is here
// because the promise the rule keeps is Parse's. Every input below returned a
// time, a nil error and Ambiguous false, and the month it named was chosen by
// the order the spelling table is written in. Which name won was not a property
// of the input, so neither was the day.
//
// The last two are the ones that matter most, because they are not fuzz
// garbage. "mar" is Tuesday in Spanish and Italian and March in English, it is
// tried before any locale name, and a Tuesday in May came back as March.
func TestParse_SecondMonthNameRefused(t *testing.T) {
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	inputs := []struct {
		input string
		opts  []Option
		was   time.Time // what it answered before
	}{
		{"MAY1MAR", nil, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"mAY1MAr", nil, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"mar 1 september 2024", nil, time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"March 15, 2024 May", nil, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"mar 15 mag 2024", []Option{WithLocales(IT)}, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"mar 15 maggio 2024", []Option{WithLocales(IT)}, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range inputs {
		opts := append([]Option{WithBaseTime(base)}, tt.opts...)
		r, err := ParseWith(tt.input, opts...)
		if err == nil {
			t.Errorf("ParseWith(%q) = %v via %s, want an error; it used to answer %v",
				tt.input, r.Time, r.Layout, tt.was)
			continue
		}
		if !errors.Is(err, ErrNoMatch) {
			t.Errorf("ParseWith(%q) error = %v, want it to unwrap to ErrNoMatch", tt.input, err)
		}
	}
}

// TestParse_SecondMonthNameKept is the regression half. A month name written
// twice is one month, an overlapping pair of spellings is one word, and a
// weekday beside a month name is RFC 2822. None of the three is two months and
// none of them may refuse.
func TestParse_SecondMonthNameKept(t *testing.T) {
	inputs := []struct {
		input string
		opts  []Option
		want  time.Time
	}{
		{"mar, 15 mar 2024", []Option{WithLocales(ES)}, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"mar 15 mar 2024", []Option{WithLocales(ES)}, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"15 de marzo de 2024", []Option{WithLocales(ES)}, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"Fri, 15 Mar 2024 10:30:00 +0000", nil, time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"sept. 1, 2020", nil, time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)},
		{"the 15th of March 2024", nil, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"invoice 15 March 2024 paid", nil, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024년 11월 11일", []Option{WithLocales(KO)}, time.Date(2024, 11, 11, 0, 0, 0, 0, time.UTC)},
		{"2024年1月11日", []Option{WithLocales(JA)}, time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range inputs {
		r, err := ParseWith(tt.input, tt.opts...)
		if err != nil {
			t.Errorf("ParseWith(%q) = %v, want %v", tt.input, err, tt.want)
			continue
		}
		if !r.Time.Equal(tt.want) {
			t.Errorf("ParseWith(%q) = %v via %s, want %v", tt.input, r.Time, r.Layout, tt.want)
		}
	}
}
