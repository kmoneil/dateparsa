package flextime

import (
	"fmt"
	"time"

	"github.com/kmoneil/dateparsa"
)

// ParseOption configures how a Scanner reads a value.
//
// It is a distinct type from MarshalOption, and being distinct is the whole
// point. There used to be one Option type covering both, and three of its four
// constructors did nothing on the value they were most likely handed to:
// NewWithOptions stored the options only when a jsonFormat was among them, and
// the three FlexTime parse paths never read them at all. So
//
//	NewWithOptions(t, WithPreferDayFirst(true))
//
// compiled, ran, and read US order on every row. Now it does not compile.
//
// A parse option cannot be honoured by a value that never parses with it. A
// FlexTime reached through encoding/json or database/sql is zero-constructed by
// the decoder, so there is no moment at which a caller could attach one, which
// is why configured parsing lives on Scanner and not on the value.
type ParseOption func(*parseConfig)

// MarshalOption configures how a FlexTime encodes itself. See ParseOption for
// why the two are separate types.
type MarshalOption func(*marshalConfig)

// parseConfig is what a Scanner is built from.
type parseConfig struct {
	preferDayFirst bool
	timezone       *time.Location
	strictMode     bool
}

// marshalConfig is what a FlexTime carries. Disjoint from parseConfig by
// construction rather than by convention: the fields cannot be confused because
// they are not in the same struct.
type marshalConfig struct {
	jsonFormat string // Go time layout for JSON output (default: RFC3339Nano)
}

func (c *parseConfig) parseOpts() []dateparsa.Option {
	var opts []dateparsa.Option
	if c.preferDayFirst {
		opts = append(opts, dateparsa.WithPreferDayFirst(true))
	}
	if c.timezone != nil {
		opts = append(opts, dateparsa.WithTimezone(c.timezone))
	}
	if c.strictMode {
		opts = append(opts, dateparsa.WithStrictMode(true))
	}
	return opts
}

// WithPreferDayFirst treats ambiguous dates as DD/MM/YYYY.
func WithPreferDayFirst(b bool) ParseOption {
	return func(c *parseConfig) { c.preferDayFirst = b }
}

// WithTimezone sets the assumed timezone for inputs without timezone info.
func WithTimezone(loc *time.Location) ParseOption {
	return func(c *parseConfig) { c.timezone = loc }
}

// WithStrictMode refuses an ambiguous date rather than guessing.
//
// A Scanner in strict mode returns dateparsa's *AmbiguousDateError, carrying
// every interpretation, for a value like "01/02/2024" that has more than one
// honest reading. errors.Is against dateparsa.ErrAmbiguous works on it, and so
// does errors.As against *dateparsa.AmbiguousDateError.
//
// This is the option SECURITY.md means by "if you are parsing dates that cross a
// trust boundary and a wrong day has consequences, use strict mode". There was no
// way to reach it from this package until now, which made that advice
// unfollowable at the boundary it names.
//
// It has no effect on a value that arrives already typed: a time.Time, an int64,
// a float64 or nil never involved a guess.
func WithStrictMode(b bool) ParseOption {
	return func(c *parseConfig) { c.strictMode = b }
}

// WithJSONFormat sets the output format for JSON marshaling.
// Default: time.RFC3339Nano.
func WithJSONFormat(layout string) MarshalOption {
	return func(c *marshalConfig) { c.jsonFormat = layout }
}

// NewWithOptions creates a FlexTime from a time.Time with marshalling
// configuration.
//
// It takes MarshalOption and not ParseOption, because a FlexTime built here is
// not the value any parse path produces: encoding/json and database/sql
// construct their own, zero-valued. Configured parsing goes through Scanner.
func NewWithOptions(t time.Time, opts ...MarshalOption) FlexTime {
	var c marshalConfig
	for _, opt := range opts {
		opt(&c)
	}
	ft := FlexTime{t: t, valid: true}
	// Stored unconditionally now. It used to be stored only when jsonFormat was
	// set, which was true of the only field that existed when the line was
	// written and silently discarded the two added after it.
	ft.opts = &c
	return ft
}

// Scanner is a pre-configured scanner for use with database rows.
// Use this when you need non-default options for database scanning.
//
// A Scanner is safe for concurrent use by multiple goroutines. It holds a
// dateparsa.Parser, which caches the last detected layout in an atomic pointer
// and is itself safe for concurrent use; the configuration behind it is fixed
// when NewScanner returns and is never written again.
//
// The cache is why this type exists. A column of one format is detected once
// and every later row runs the compiled layout, which is the difference
// between about 160 ns and about 30 ns a row. A column whose format changes
// partway through still parses correctly: a layout that does not fit the row
// fails and detection runs again.
//
// It is also the only configured parse path in this package. See ParseOption.
type Scanner struct {
	p *dateparsa.Parser
}

// NewScanner creates a Scanner with the given options.
func NewScanner(opts ...ParseOption) *Scanner {
	var c parseConfig
	for _, opt := range opts {
		opt(&c)
	}
	return &Scanner{p: dateparsa.NewParser(c.parseOpts()...)}
}

// Scan parses src into the provided FlexTime using the scanner's configuration.
func (s *Scanner) Scan(ft *FlexTime, src any) error {
	switch v := src.(type) {
	case nil:
		ft.set(time.Time{}, false, false)
		return nil

	case time.Time:
		ft.set(v, true, false)
		return nil

	case string:
		return s.scanString(ft, v)

	case []byte:
		return s.scanString(ft, string(v))

	case int64, float64:
		// Delegated rather than duplicated. The int64 arm used to be its own copy
		// of time.Unix(v, 0) here, so it kept reading a millisecond epoch as
		// seconds after FlexTime.Scan stopped: one bug fixed in one of the two
		// places that had it. Neither numeric form is affected by any option a
		// Scanner carries, so there is nothing this arm could add.
		return ft.Scan(v)

	default:
		return ft.Scan(src) // delegate for unsupported type error
	}
}

func (s *Scanner) scanString(ft *FlexTime, str string) error {
	if str == "" {
		return ft.Scan(str) // delegate for error
	}
	result, err := s.p.Parse(str)
	if err != nil {
		// Report the configured parse's own failure. This used to call
		// ft.Scan(str), which re-parses with default options "for error
		// message": if the default parse ever succeeded where the configured
		// one failed, a configured Scanner returned a default-configured
		// result and no error. That is reachable now rather than merely
		// shapely, because strict mode changes what Parse *refuses* and not
		// only what it returns.
		return fmt.Errorf("flextime: cannot scan %q: %w", str, err)
	}
	ft.set(result.Time, true, result.Ambiguous)
	return nil
}
