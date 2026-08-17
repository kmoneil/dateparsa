package dateparsa

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseError_ErrorsIs_ErrNoMatch(t *testing.T) {
	_, err := Parse("not a date")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("errors.Is(err, ErrNoMatch) = false; err = %v", err)
	}
}

func TestParseError_ErrorsAs(t *testing.T) {
	_, err := Parse("not a date")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As failed; err type = %T", err)
	}
	if pe.Input != "not a date" {
		t.Errorf("ParseError.Input = %q, want %q", pe.Input, "not a date")
	}
}

func TestAmbiguousDateError_ErrorsIs_ErrAmbiguous(t *testing.T) {
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	if err == nil {
		t.Fatal("expected error in strict mode")
	}
	if !errors.Is(err, ErrAmbiguous) {
		t.Errorf("errors.Is(err, ErrAmbiguous) = false; err = %v", err)
	}
}

func TestAmbiguousDateError_ErrorsAs(t *testing.T) {
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	if err == nil {
		t.Fatal("expected error in strict mode")
	}
	var ae *AmbiguousDateError
	if !errors.As(err, &ae) {
		t.Fatalf("errors.As failed; err type = %T", err)
	}
	if ae.Input != "01/02/2024" {
		t.Errorf("AmbiguousDateError.Input = %q", ae.Input)
	}
}

func TestParseError_Unwrap_Nil(t *testing.T) {
	// A ParseError with no Cause should unwrap to nil.
	pe := &ParseError{Input: "x", Message: "test"}
	if pe.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil", pe.Unwrap())
	}
}

// TestParseError_MessageIsBounded is the gate on the one thing this library
// does with a hostile input besides refusing it: hand it back in an error.
//
// A megabyte of junk used to produce a 1,048,628-byte message, and a megabyte
// through flextime produced 2,097,231, because the wrapper quoted the input a
// second time. Both are linear rather than amplifying, which is why this is a
// bound and not a crash, but the error outlives the call and something holds it.
func TestParseError_MessageIsBounded(t *testing.T) {
	const big = 1 << 20
	input := strings.Repeat("x", big)

	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected an error for a megabyte of junk")
	}
	// The prefix, the quoted 64 bytes, the byte count, and the message. Well
	// under 256 even if every quoted byte needed a \xNN escape.
	if n := len(err.Error()); n > 512 {
		t.Errorf("Error() is %d bytes for a %d-byte input, want <= 512", n, big)
	}
	if !strings.Contains(err.Error(), "1048576 bytes") {
		t.Errorf("Error() does not name the true length: %s", err.Error())
	}

	// The field is not truncated: a caller that wants the input has it.
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As failed; err type = %T", err)
	}
	if len(pe.Input) != big {
		t.Errorf("ParseError.Input is %d bytes, want the whole %d", len(pe.Input), big)
	}
}

// A short input is quoted whole, which is every real error message.
func TestParseError_ShortInputIsWhole(t *testing.T) {
	_, err := Parse("not a date")
	if err == nil {
		t.Fatal("expected error")
	}
	const want = `dateparsa: cannot parse "not a date": no matching format found`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAmbiguousDateError_MessageIsBounded(t *testing.T) {
	// Constructed rather than parsed. Nothing detects an ambiguous date out of
	// a megabyte today, since padding that long fails detection outright and
	// returns a *ParseError, but Input is a public field on a public type and
	// the caller of Error() should not have to know which of the two they hold.
	ae := &AmbiguousDateError{
		Input:           strings.Repeat("1/2/03 ", 1000),
		Interpretations: []Interpretation{{Label: "MM/DD/YYYY"}, {Label: "DD/MM/YYYY"}},
	}
	if n := len(ae.Error()); n > 512 {
		t.Errorf("Error() is %d bytes for a %d-byte input, want <= 512", n, len(ae.Input))
	}
	if !strings.Contains(ae.Error(), "7000 bytes") {
		t.Errorf("Error() does not name the true length: %s", ae.Error())
	}
}

// TestQuoteInput_ClipsToRuneBoundary: a cut through a multi-byte rune renders
// as \xNN, which reads as corrupt input rather than as truncation.
func TestQuoteInput_ClipsToRuneBoundary(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			// 64 ASCII bytes: the boundary itself, quoted whole.
			name:  "exactly the limit",
			input: strings.Repeat("a", maxErrInput),
			want:  `"` + strings.Repeat("a", maxErrInput) + `"`,
		},
		{
			name:  "one over",
			input: strings.Repeat("a", maxErrInput+1),
			want:  `"` + strings.Repeat("a", maxErrInput) + `"... (65 bytes)`,
		},
		{
			// 63 ASCII then a 2-byte rune: the cut lands inside it, so the
			// whole rune goes.
			name:  "cut inside a 2-byte rune",
			input: strings.Repeat("a", 63) + "ç" + "zzzz",
			want:  `"` + strings.Repeat("a", 63) + `"... (69 bytes)`,
		},
		{
			// 62 ASCII then a 3-byte rune, cut after its second byte.
			name:  "cut inside a 3-byte rune",
			input: strings.Repeat("a", 62) + "月" + "zzzz",
			want:  `"` + strings.Repeat("a", 62) + `"... (69 bytes)`,
		},
		{
			// 61 ASCII then a 4-byte rune, cut after its third byte.
			name:  "cut inside a 4-byte rune",
			input: strings.Repeat("a", 61) + "𝟚" + "zzzz",
			want:  `"` + strings.Repeat("a", 61) + `"... (69 bytes)`,
		},
		{
			// A rune that ends exactly on the boundary is kept whole.
			name:  "rune ends on the boundary",
			input: strings.Repeat("a", 62) + "ç" + "zzzz",
			want:  `"` + strings.Repeat("a", 62) + `ç"... (68 bytes)`,
		},
		{
			// Not UTF-8 at all: continuation bytes with no lead byte in reach.
			// The cut stays where it is and %q shows them.
			name:  "invalid utf-8 is not clipped away",
			input: strings.Repeat("a", 60) + strings.Repeat("\x80", 10),
			want:  `"` + strings.Repeat("a", 60) + `\x80\x80\x80\x80"... (70 bytes)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteInput(tt.input); got != tt.want {
				t.Errorf("quoteInput() =\n\t%s\nwant\n\t%s", got, tt.want)
			}
		})
	}
}

func TestLocale_ZeroValue_String(t *testing.T) {
	var loc Locale
	if s := loc.String(); s != "" {
		t.Errorf("zero Locale.String() = %q, want empty", s)
	}
}

func TestWithTimezone_Nil(t *testing.T) {
	// WithTimezone(nil) should behave like UTC, not panic.
	result, err := ParseWith("2024-03-15", WithTimezone(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Location() != time.UTC {
		t.Errorf("got location %v, want UTC", result.Time.Location())
	}
}

func TestWithStrictMode_Toggle(t *testing.T) {
	// Strict mode can be enabled and disabled.
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	if err == nil {
		t.Fatal("expected error with strict mode on")
	}
	_, err = ParseWith("01/02/2024", WithStrictMode(false))
	if err != nil {
		t.Fatalf("unexpected error with strict mode off: %v", err)
	}
}

func TestLocales_Sorted(t *testing.T) {
	tags := Locales()
	for i := 1; i < len(tags); i++ {
		if tags[i] < tags[i-1] {
			t.Errorf("Locales() not sorted: %q before %q", tags[i-1], tags[i])
			break
		}
	}
}
