package dateparsa

import (
	"fmt"
	"time"
)

// ParseError is returned when a date string cannot be parsed.
type ParseError struct {
	Input   string
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("dateparsa: cannot parse %q: %s", e.Input, e.Message)
}

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
