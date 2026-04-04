package dateparsa

import (
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
)

// Compile parses a Go time reference layout string (e.g. "2006-01-02")
// and returns a *Layout that uses dateparsa's instruction executor.
// Returns an error if the layout contains unsupported reference tokens
// (month/day names like "Jan", "January", "Mon" require auto-detection
// via Parse).
//
// The returned Layout is safe for concurrent use and produces identical
// results to time.Parse(layout, value) for all valid inputs.
func Compile(layout string) (*Layout, error) {
	return CompileWithTimezone(layout, time.UTC)
}

// MustCompile is like Compile but panics on error.
// Intended for use in package-level variable initialization.
//
//	var myLayout = dateparsa.MustCompile("2006-01-02")
func MustCompile(layout string) *Layout {
	l, err := Compile(layout)
	if err != nil {
		panic("dateparsa.MustCompile: " + err.Error())
	}
	return l
}

// CompileWithTimezone is like Compile but sets a default timezone
// for inputs that lack timezone information.
func CompileWithTimezone(layout string, tz *time.Location) (*Layout, error) {
	def, err := compile.ParseGoLayout(layout)
	if err != nil {
		return nil, err
	}

	program := compile.Compile(def, tz)
	return &Layout{
		program:  program,
		goLayout: def.GoLayout,
		label:    def.Name,
	}, nil
}
