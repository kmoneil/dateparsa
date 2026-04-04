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
type ParseError struct {
	Input   string
	Message string
	Cause   error // underlying sentinel, if any; supports errors.Is
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("dateparsa: cannot parse %q: %s", e.Input, e.Message)
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
type AmbiguousDateError struct {
	Input           string
	Interpretations []Interpretation
}

func (e *AmbiguousDateError) Error() string {
	return fmt.Sprintf("dateparsa: ambiguous date %q has %d interpretations", e.Input, len(e.Interpretations))
}

func (e *AmbiguousDateError) Unwrap() error { return ErrAmbiguous }
