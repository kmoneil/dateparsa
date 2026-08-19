package dateparsa

import (
	"testing"
	"time"

	"github.com/kmoneil/dateparsa/internal/locale"
)

var localeBase = time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

// === Structured dates with localized month names ===

func TestParse_Locale_FrenchMonths(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"15 mars 2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"15 janvier 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
		{"1 décembre 2023", time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)},
		{"15 janv 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(FR))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_GermanMonths(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"15 März 2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"1 Januar 2024", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"31 Dezember 2023", time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(DE))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_SpanishMonths(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"15 marzo 2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"1 enero 2024", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"31 diciembre 2023", time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(ES))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_ItalianMonths(t *testing.T) {
	result, err := ParseWith("15 marzo 2024", WithLocales(IT))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_Locale_PortugueseMonths(t *testing.T) {
	result, err := ParseWith("15 março 2024", WithLocales(PT))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_Locale_DutchMonths(t *testing.T) {
	result, err := ParseWith("15 maart 2024", WithLocales(NL))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_Locale_RussianMonths(t *testing.T) {
	result, err := ParseWith("15 марта 2024", WithLocales(RU))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_Locale_PolishMonths(t *testing.T) {
	result, err := ParseWith("15 marca 2024", WithLocales(PL))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_Locale_TurkishMonths(t *testing.T) {
	result, err := ParseWith("15 Mart 2024", WithLocales(TR))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === Natural language with locales ===

func TestParse_Locale_FrenchNL(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"hier", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"demain", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"aujourd'hui", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(FR), WithBaseTime(localeBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_GermanNL(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"gestern", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"morgen", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"heute", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(DE), WithBaseTime(localeBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_SpanishNL(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"ayer", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"mañana", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"hoy", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(ES), WithBaseTime(localeBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

func TestParse_Locale_RussianNL(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
	}{
		{"вчера", time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)},
		{"завтра", time.Date(2024, 3, 16, 0, 0, 0, 0, time.UTC)},
		{"сегодня", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseWith(tt.input, WithLocales(RU), WithBaseTime(localeBase))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if !result.Time.Equal(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, result.Time, tt.expected)
			}
		})
	}
}

// === Public API ===

func TestLookupLocale(t *testing.T) {
	tests := []struct {
		tag   string
		found bool
	}{
		{"en", true},
		{"fr", true},
		{"de", true},
		{"es", true},
		{"ja", true},
		{"zh", true},
		{"xx", false},
		{"", false},
	}
	for _, tt := range tests {
		_, ok := LookupLocale(tt.tag)
		if ok != tt.found {
			t.Errorf("LookupLocale(%q) = _, %v, want %v", tt.tag, ok, tt.found)
		}
	}
}

func TestLocales(t *testing.T) {
	tags := Locales()
	if len(tags) < 20 {
		t.Errorf("expected at least 20 locales, got %d", len(tags))
	}
}

func TestLocale_String(t *testing.T) {
	if FR.String() != "fr" {
		t.Errorf("FR.String() = %q, want fr", FR.String())
	}
}

// === Benchmarks ===

func BenchmarkParse_Locale_FrenchMonth(b *testing.B) {
	opts := []Option{WithLocales(FR)}
	for b.Loop() {
		ParseWith("15 mars 2024", opts...)
	}
}

func BenchmarkParse_Locale_FrenchNL(b *testing.B) {
	opts := []Option{WithLocales(FR), WithBaseTime(localeBase)}
	for b.Loop() {
		ParseWith("hier", opts...)
	}
}

func BenchmarkParse_Locale_GermanMonth(b *testing.B) {
	opts := []Option{WithLocales(DE)}
	for b.Loop() {
		ParseWith("15 März 2024", opts...)
	}
}

// BenchmarkParse_Locale_RussianNL is a relative expression in a non-Latin
// script, which is the weak case for bucketing the phrase table on the first
// byte of the phrase: all 60 Russian phrases start with one of two UTF-8 lead
// bytes, so a Russian word still measures against about half the table where a
// French word measures against a tenth of the French one.
func BenchmarkParse_Locale_RussianNL(b *testing.B) {
	opts := []Option{WithLocales(RU), WithBaseTime(localeBase)}
	for b.Loop() {
		ParseWith("3 дня назад", opts...)
	}
}

// Every month name of every registered locale, through the public API.
//
// This is the test that was missing. phase4_test.go had month-name cases for 9
// of the 20 locales and relative-expression cases for 4, so Arabic, Danish,
// Finnish, Korean, Norwegian, Swedish and Ukrainian were referenced by no test
// at all, and the CJK locales were tested only in the year月day form.
//
// What it found: buildLocaleMonths listed spellings in month order, so "1월"
// was tried before "11월" and Korean November parsed as January. A month name
// beginning with digits is what makes that reachable, and every CJK locale
// spells its months that way, so ja, ko and zh all had months 11 and 12 wrong.
//
// Written as a loop over locale.Tags() rather than a table, so a locale added
// later is covered by having been added rather than by somebody remembering.
func TestParse_EveryLocaleMonthName(t *testing.T) {
	for _, tag := range Locales() {
		loc, ok := LookupLocale(tag)
		if !ok {
			t.Errorf("%s is in Locales() and LookupLocale does not know it", tag)
			continue
		}
		d := locale.Lookup(tag)
		for i := range 12 {
			for _, name := range []string{d.MonthsWide[i], d.MonthsAbbr[i]} {
				if name == "" {
					continue
				}
				in := "15 " + name + " 2024"
				r, err := ParseWith(in, WithLocales(loc))
				if err != nil {
					t.Errorf("%s: ParseWith(%q) failed: %v", tag, in, err)
					continue
				}
				if got := int(r.Time.Month()); got != i+1 {
					t.Errorf("%s: ParseWith(%q) read month %d, want %d (%s)",
						tag, in, got, i+1, r.Time.Format("2006-01-02"))
				}
				if r.Time.Day() != 15 || r.Time.Year() != 2024 {
					t.Errorf("%s: ParseWith(%q) = %s, want 2024-%02d-15",
						tag, in, r.Time.Format("2006-01-02"), i+1)
				}
			}
		}
	}
}

// The forms a reader would actually write, for the three locales whose months
// are digits and a character. The loop above uses one synthetic shape for every
// locale; these are the real ones, and 11 and 12 are the months that were wrong.
func TestParse_CJKNativeMonths(t *testing.T) {
	tests := []struct {
		tag, in string
		month   int
	}{
		{"ja", "2024年1月15日", 1},
		{"ja", "2024年11月15日", 11},
		{"ja", "2024年12月15日", 12},
		{"zh", "2024年11月15日", 11},
		{"zh", "2024年12月15日", 12},
		{"ko", "2024년 1월 15일", 1},
		{"ko", "2024년 11월 15일", 11},
		{"ko", "2024년 12월 15일", 12},
	}
	for _, tc := range tests {
		loc, _ := LookupLocale(tc.tag)
		r, err := ParseWith(tc.in, WithLocales(loc))
		if err != nil {
			t.Errorf("%s: ParseWith(%q) failed: %v", tc.tag, tc.in, err)
			continue
		}
		if got := int(r.Time.Month()); got != tc.month {
			t.Errorf("%s: ParseWith(%q) read month %d, want %d", tc.tag, tc.in, got, tc.month)
		}
	}
}
