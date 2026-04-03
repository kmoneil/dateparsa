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

// Parse parses a date string using this pre-detected layout.
// Skips format detection entirely. This is the hot path.
func (l *Layout) Parse(s string) (time.Time, error) {
	return l.program.Execute(s)
}

// ParseBytes is like Parse but operates on a byte slice.
func (l *Layout) ParseBytes(b []byte) (time.Time, error) {
	return l.program.ExecuteBytes(b)
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
