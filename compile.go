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
// The returned Layout is safe for concurrent use. Where an input is what the
// layout literally describes, it agrees with time.Parse(layout, value).
//
// It is stricter in one respect, deliberately. A compiled layout reads fields
// at fixed byte offsets, which is what lets it run without allocating, so a
// numeric element written narrower than the layout declares is refused here:
// Compile("2006-01-02T15:04:05Z07:00") rejects "2024-03-15T9:30:00Z", while
// time.Parse accepts it by falling back from its strict RFC 3339 parser to the
// general layout parser, where 15 takes one digit or two. RFC 3339 requires
// two. Use Parse if you want the width detected rather than declared.
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
