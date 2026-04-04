package dateparsa

import (
	"errors"
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
