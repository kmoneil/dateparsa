package dateparsa

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for use with errors.Is.
var (
	// ErrNoMatch is returned when no format matches the input.
	ErrNoMatch = errors.New("dateparsa: no matching format")

	// ErrAmbiguous is returned when the input matches multiple formats
	// and strict mode is enabled.
	ErrAmbiguous = errors.New("dateparsa: ambiguous date")
)

// ParseError is returned when a date string cannot be parsed.
//
// Input is whatever was passed in, at whatever length. Error() quotes at most
// 64 bytes of it and names the rest by size: the input is assumed
// hostile, and an error message is one of the two places it comes back out, so
// a megabyte in must not mean a megabyte of message retained for as long as
// something holds the error. Read Input for the whole string.
type ParseError struct {
	Input   string
	Message string
	Cause   error // underlying sentinel, if any; supports errors.Is
}

func (e *ParseError) Error() string {
	return "dateparsa: cannot parse " + quoteInput(e.Input) + ": " + e.Message
}

func (e *ParseError) Unwrap() error { return e.Cause }

// Interpretation is one possible reading of an ambiguous date.
type Interpretation struct {
	Time   time.Time
	Layout *Layout
	Label  string // e.g. "MM/DD/YYYY", "DD/MM/YYYY"
}

// AmbiguousDateError is returned in strict mode when the format
// is ambiguous (e.g. "01/02/03").
//
// Input and Error() relate the same way they do on ParseError: the field is
// whole, the message is bounded.
type AmbiguousDateError struct {
	Input           string
	Interpretations []Interpretation
}

func (e *AmbiguousDateError) Error() string {
	return fmt.Sprintf("dateparsa: ambiguous date %s has %d interpretations",
		quoteInput(e.Input), len(e.Interpretations))
}

func (e *AmbiguousDateError) Unwrap() error { return ErrAmbiguous }

// maxErrInput bounds how many bytes of the input an error message quotes.
//
// Everything this library can parse is far shorter than this: the longest
// format in the round-trip suite renders 33 bytes, a compiled program describes
// at most compile.MaxDescribableLen, and the natural-language scanner refuses
// anything over natural.MaxInputLen. So a message that truncates is a message
// about an input that was never going to parse, and its first 64 bytes are what
// identifies it.
const maxErrInput = 64

// quoteInput renders s for an error message: %q of the whole string when it is
// short enough, and %q of a prefix plus the true length when it is not.
func quoteInput(s string) string {
	if len(s) <= maxErrInput {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%q... (%d bytes)", s[:clipRune(s, maxErrInput)], len(s))
}

// clipRune returns the largest n <= max such that s[:n] does not end in the
// middle of a UTF-8 rune. It assumes len(s) > max.
//
// This is not decoration. A date in a language this library ships locale data
// for is multi-byte, and a cut through "15 de março" would render its tail as
// \xc3, which reads as an encoding bug in the caller's data rather than as the
// truncation it is. A byte that is not part of a rune at all is left where it
// is: the threat model says the input may be invalid UTF-8, and %q showing that
// honestly is the point.
func clipRune(s string, max int) int {
	if s[max]&0xC0 != 0x80 {
		return max // the cut lands on a rune start, or on a byte that is not one
	}
	// s[max] continues a rune that began earlier. A rune is at most 4 bytes, so
	// its first byte is at most 3 back; further than that and this is not UTF-8
	// and the cut stays where it was.
	for i := max - 1; i >= 0 && max-i <= 3; i-- {
		if s[i]&0xC0 != 0x80 {
			return i
		}
	}
	return max
}
