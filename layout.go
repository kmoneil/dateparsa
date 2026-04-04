package dateparsa

import (
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
)

// Layout is a compiled date format detected from a sample string.
// Safe for concurrent use. Immutable after detection.
// Reusing a Layout skips format detection entirely.
type Layout struct {
	program  compile.Program
	goLayout string // Go time layout equivalent, if any
	label    string // Human-readable label, e.g. "ISO8601_DATE"
}

// Sentinel layouts for non-structured parse results.
// These are returned by Parse/ParseWith for inputs that don't have a
// reusable format (epoch timestamps and natural language expressions).
// Calling Parse or ParseBytes on a sentinel layout returns a *ParseError.
var (
	LayoutEpoch           = &Layout{label: "UNIX_TIMESTAMP"}
	LayoutNaturalLanguage = &Layout{label: "NATURAL_LANGUAGE"}
)

// Parse parses a date string using this pre-detected layout.
// Skips format detection entirely. This is the hot path.
//
// Returns a *ParseError if the input does not match the layout,
// or if this is a sentinel layout (LayoutEpoch, LayoutNaturalLanguage).
func (l *Layout) Parse(s string) (time.Time, error) {
	if l.program.N == 0 {
		return time.Time{}, &ParseError{
			Input:   s,
			Message: "cannot re-parse with " + l.label + " layout",
		}
	}
	t, err := l.program.Execute(s)
	if err != nil {
		return time.Time{}, &ParseError{Input: s, Message: err.Error()}
	}
	return t, nil
}

// ParseBytes is like Parse but operates on a byte slice.
func (l *Layout) ParseBytes(b []byte) (time.Time, error) {
	if l.program.N == 0 {
		return time.Time{}, &ParseError{
			Input:   string(b),
			Message: "cannot re-parse with " + l.label + " layout",
		}
	}
	t, err := l.program.ExecuteBytes(b)
	if err != nil {
		return time.Time{}, &ParseError{Input: string(b), Message: err.Error()}
	}
	return t, nil
}

// String returns a human-readable label for this layout.
func (l *Layout) String() string {
	return l.label
}

// GoLayout returns the equivalent Go time.Layout constant if applicable.
// Returns ("", false) if no stdlib equivalent exists.
func (l *Layout) GoLayout() (string, bool) {
	if l.goLayout == "" {
		return "", false
	}
	return l.goLayout, true
}
