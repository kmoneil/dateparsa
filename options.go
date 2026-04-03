package dateparsa

import "time"

// config holds the parsed options for a parse call.
type config struct {
	baseTime       time.Time
	timezone       *time.Location
	preferDayFirst bool
	preferYearFirst bool
	preferFuture    bool
	strictMode      bool
	locales         []Locale
}

func buildConfig(opts []Option) config {
	c := config{timezone: time.UTC}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Option configures parsing behavior.
type Option func(*config)

// WithBaseTime sets the reference time for relative date expressions.
// Default: time.Now().
func WithBaseTime(t time.Time) Option {
	return func(c *config) { c.baseTime = t }
}

// WithTimezone sets the assumed timezone when the input has no
// timezone indicator. Default: time.UTC.
func WithTimezone(loc *time.Location) Option {
	return func(c *config) { c.timezone = loc }
}

// WithPreferDayFirst treats ambiguous dates as DD/MM/YYYY.
// Default: false (MM/DD/YYYY, American convention).
func WithPreferDayFirst(b bool) Option {
	return func(c *config) { c.preferDayFirst = b }
}

// WithPreferYearFirst treats ambiguous dates as YYYY/MM/DD.
// Takes precedence over PreferDayFirst when applicable.
func WithPreferYearFirst(b bool) Option {
	return func(c *config) { c.preferYearFirst = b }
}

// WithPreferFuture makes relative dates without direction default
// to the future. Default: false (prefer past).
func WithPreferFuture(b bool) Option {
	return func(c *config) { c.preferFuture = b }
}

// WithStrictMode rejects ambiguous dates instead of applying
// preference rules. Returns an *AmbiguousDateError.
func WithStrictMode() Option {
	return func(c *config) { c.strictMode = true }
}
