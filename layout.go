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
//
// They are compared by identity and must not be reassigned. That is the same
// contract io.EOF and time.UTC carry, for the same reason: Go has no immutable
// package-level pointer, so a sentinel is a var and the convention does the
// work. Assigning nil to one of these makes Parse return a ParseResult whose
// Layout is nil, and the panic lands in the caller's code on the line that
// reuses it.
//
// Prefer Reusable to comparing against these. It answers the question a caller
// actually has, and it stays right if a third sentinel is ever added.
var (
	LayoutEpoch           = &Layout{label: "UNIX_TIMESTAMP"}
	LayoutNaturalLanguage = &Layout{label: "NATURAL_LANGUAGE"}
)

// Reusable reports whether this Layout can parse another input.
//
// It is false for the sentinels above, which describe a result rather than a
// format: an epoch timestamp has no format to reuse, and "3 days ago" resolves
// against the time it was parsed at, so re-running it later against a different
// base would answer a different day from a value the caller believes is a
// compiled layout. Both refuse rather than answering, and this is how to ask
// before finding out.
//
// A nil Layout is not reusable rather than a panic, because the point of this
// method is to be the check a caller makes before using what they were handed.
//
//	result, err := dateparsa.Parse(s)
//	if err == nil && result.Layout.Reusable() {
//	    // keep it for the rest of the column
//	}
//
// It exists because the alternative was writing one. The comparison benchmark
// in this repository, which is a separate module and therefore an ordinary
// caller, had a helper that called Parse("") and then string-compared
// Layout.String() against "UNIX_TIMESTAMP" and "NATURAL_LANGUAGE" to work this
// out.
func (l *Layout) Reusable() bool {
	return l != nil && l.program.N != 0
}

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

// GoLayout returns the equivalent Go time layout, and reports whether there is
// one. Returns ("", false) for a format with no stdlib equivalent, which is
// every format the fallback detectors produce: the textual months, the
// variable-width numeric ones and the Go time string.
//
// The layout it returns parses the input this Layout was detected from. That is
// not free and it is worth knowing why: a trie entry is keyed by a sequence of
// character classes rather than of bytes, so one entry matches "2024-03-15",
// "2024/03/15" and "2024.03.15", and the entry names one spelling. The layout
// is respelled to the input's separators at detection, so a caller who hands it
// to time.Parse for the next row of their column gets one that reads it.
//
// It describes the input, not everything this Layout accepts. Layout.Parse
// takes all three spellings; the returned Go layout takes the one that was
// detected.
func (l *Layout) GoLayout() (string, bool) {
	if l.goLayout == "" {
		return "", false
	}
	return l.goLayout, true
}
