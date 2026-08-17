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

// WithPreferYearFirst is accepted and does nothing.
//
// It is meant to read an ambiguous three-part numeric date as YYYY/MM/DD, and
// no detector consults it: detect.Config carries the field, four call sites set
// it, and nothing reads it, so "01/02/03" is the second of January 2003 with
// the option on and with it off. It is documented rather than removed because
// the behaviour is wanted, and it is not a one-liner: with a year-first reading
// available, "01/02/03" has three honest readings and an *AmbiguousDateError
// carries a pair, so what strict mode returns has to be decided first.
//
// Do not write code that depends on this option until this comment says it
// works.
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
