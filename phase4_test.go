package dateparsa

import (
	"testing"
	"time"
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
