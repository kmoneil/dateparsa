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

	// ambiguous records that detection had to guess which field was which,
	// as for DD/MM against MM/DD. It travels with the layout because Parser
	// reuses the layout without re-detecting, and a guess that was reported
	// once has to stay reported every time the guess is reused.
	//
	// Set at construction and never afterwards: Layout is documented as
	// immutable and safe for concurrent use, and this field is part of that.
	ambiguous bool

	// ambiguityProne records that the format can need a guess for some input,
	// whether or not the input it was detected from did. Ambiguity belongs to
	// the input, so a cached layout cannot answer it for the next value:
	// "25/12/2024" resolves without a guess and yields a layout that meets
	// "01/02/2024" two rows later. Strict mode declines the cache on this.
	ambiguityProne bool
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
// It allocates nothing, which is the reason this type exists, with one
// exception worth knowing before it surprises you. A timezone offset is
// answered from a table of Locations built at init, and an offset that is not
// on a 15-minute boundary or is past 14 hours is not in that table: the first
// input carrying such an offset builds its Location and caches it, three
// allocations, and every later input at the same offset allocates nothing. So a
// column of pre-1955 Indian records, which carry +05:53, allocates three times
// in total rather than three times per row.
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
