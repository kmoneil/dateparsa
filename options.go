package dateparsa

import "time"

// config holds the parsed options for a parse call.
type config struct {
	baseTime        time.Time
	timezone        *time.Location
	preferDayFirst  bool
	preferYearFirst bool
	preferFuture    bool
	strictMode      bool
	locales         []Locale
}

func buildConfig(opts []Option) config {
	c := config{timezone: time.UTC}
	for _, o := range opts {
		c = o(c)
	}
	return c
}

// Option configures parsing behavior. Build one with a With function;
// config is unexported, so this package is the only place an Option
// can come from.
//
// An Option takes a config by value and returns the modified copy. It
// does not take a pointer: handing the address of a local config to an
// opaque func value forces that config onto the heap, which cost every
// ParseWith, ParseTime and Detect call one 64-byte allocation.
type Option func(config) config

// WithBaseTime sets the reference time for relative date expressions.
// Default: time.Now().
func WithBaseTime(t time.Time) Option {
	return func(c config) config {
		c.baseTime = t
		return c
	}
}

// WithTimezone sets the assumed timezone when the input has no
// timezone indicator. Default: time.UTC.
func WithTimezone(loc *time.Location) Option {
	return func(c config) config {
		if loc == nil {
			loc = time.UTC
		}
		c.timezone = loc
		return c
	}
}

// WithPreferDayFirst treats ambiguous dates as DD/MM/YYYY.
// Default: false (MM/DD/YYYY, American convention).
func WithPreferDayFirst(b bool) Option {
	return func(c config) config {
		c.preferDayFirst = b
		return c
	}
}

// WithPreferYearFirst reads a three-part numeric date as YY/MM/DD when the
// input leaves the year's position open. Default: false, the year is last.
//
// It applies to a date whose three parts are all small, because that is the
// only shape where nothing in the input says which end the year is at:
// "01/02/03" is 2001-02-03 with this on and 2003-01-02 with it off. A written
// four-digit year wins over it, so "01/02/2024" is unaffected, and so does a
// value that cannot be what the reading needs: "01/13/03" has no month 13, so
// the year stays last and the option stands aside rather than refusing the row.
//
// Year-first means year, then month, then day. Every format that writes the
// year first writes ISO order after it, so WithPreferDayFirst does not
// participate in the reading this option selects.
//
// The result is reported as ambiguous, because it is: the year-last readings
// still exist, and strict mode returns all three rather than the two every
// other ambiguous input has.
func WithPreferYearFirst(b bool) Option {
	return func(c config) config {
		c.preferYearFirst = b
		return c
	}
}

// WithPreferFuture makes relative dates without direction default
// to the future. Default: false (prefer past).
func WithPreferFuture(b bool) Option {
	return func(c config) config {
		c.preferFuture = b
		return c
	}
}

// WithStrictMode controls whether ambiguous dates are rejected.
// When true, ambiguous dates return an *AmbiguousDateError instead
// of applying preference rules.
func WithStrictMode(b bool) Option {
	return func(c config) config {
		c.strictMode = b
		return c
	}
}
