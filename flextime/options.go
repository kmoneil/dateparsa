package flextime

import (
	"fmt"
	"time"

	"github.com/kmoneil/dateparsa"
)

// Option configures FlexTime behavior.
type Option func(*options)

type options struct {
	preferDayFirst bool
	timezone       *time.Location
	jsonFormat     string // Go time layout for JSON output (default: RFC3339Nano)
}

func (o *options) parseOpts() []dateparsa.Option {
	var opts []dateparsa.Option
	if o.preferDayFirst {
		opts = append(opts, dateparsa.WithPreferDayFirst(true))
	}
	if o.timezone != nil {
		opts = append(opts, dateparsa.WithTimezone(o.timezone))
	}
	return opts
}

// WithPreferDayFirst treats ambiguous dates as DD/MM/YYYY.
func WithPreferDayFirst(b bool) Option {
	return func(o *options) { o.preferDayFirst = b }
}

// WithTimezone sets the assumed timezone for inputs without timezone info.
func WithTimezone(loc *time.Location) Option {
	return func(o *options) { o.timezone = loc }
}

// WithJSONFormat sets the output format for JSON marshaling.
// Default: time.RFC3339Nano.
func WithJSONFormat(layout string) Option {
	return func(o *options) { o.jsonFormat = layout }
}

// NewWithOptions creates a FlexTime from a time.Time with configuration.
func NewWithOptions(t time.Time, opts ...Option) FlexTime {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	ft := FlexTime{t: t, valid: true}
	if o.jsonFormat != "" {
		ft.opts = &o
	}
	return ft
}

// Scanner is a pre-configured scanner for use with database rows.
// Use this when you need non-default options for database scanning.
type Scanner struct {
	opts options
}

// NewScanner creates a Scanner with the given options.
func NewScanner(opts ...Option) *Scanner {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Scanner{opts: o}
}

// Scan parses src into the provided FlexTime using the scanner's configuration.
func (s *Scanner) Scan(ft *FlexTime, src interface{}) error {
	switch v := src.(type) {
	case nil:
		ft.t = time.Time{}
		ft.valid = false
		return nil

	case time.Time:
		ft.t = v
		ft.valid = true
		return nil

	case string:
		return s.scanString(ft, v)

	case []byte:
		return s.scanString(ft, string(v))

	case int64:
		ft.t = time.Unix(v, 0)
		ft.valid = true
		return nil

	case float64:
		return ft.Scan(v) // delegate to FlexTime.Scan for float64

	default:
		return ft.Scan(src) // delegate for unsupported type error
	}
}

func (s *Scanner) scanString(ft *FlexTime, str string) error {
	if str == "" {
		return ft.Scan(str) // delegate for error
	}
	parseOpts := s.opts.parseOpts()
	result, err := dateparsa.ParseWith(str, parseOpts...)
	if err != nil {
		// Report the configured parse's own failure. This used to call
		// ft.Scan(str), which re-parses with default options "for error
		// message": if the default parse ever succeeded where the configured
		// one failed, a configured Scanner returned a default-configured
		// result and no error. No input reaches that today, because
		// PreferDayFirst and Timezone change what Parse returns rather than
		// what it refuses, so this is the shape being closed and not a bug
		// being fixed.
		return fmt.Errorf("flextime: cannot scan %q: %w", str, err)
	}
	ft.t = result.Time
	ft.valid = true
	return nil
}
