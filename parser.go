package dateparsa

import "time"

// Parser is a stateful parser optimized for parsing many dates
// with the same (or similar) format. It caches the last successful
// Layout and tries it first on subsequent calls.
//
// A Parser is not safe for concurrent use by multiple goroutines.
// To parse concurrently, create a separate Parser per goroutine,
// or use Layout.Parse directly for the concurrent hot path.
type Parser struct {
	cfg    config
	layout *Layout
}

// NewParser creates a parser with the given options.
func NewParser(opts ...Option) *Parser {
	return &Parser{cfg: buildConfig(opts)}
}

// Parse parses a date string. If the previously detected Layout
// succeeds, it skips detection entirely. Falls back to full
// detection on format change.
func (p *Parser) Parse(s string) (ParseResult, error) {
	// Fast path: try the cached layout first.
	if p.layout != nil {
		t, err := p.layout.Parse(s)
		if err == nil {
			return ParseResult{
				Time:   t,
				Layout: p.layout,
				Kind:   KindAbsolute,
			}, nil
		}
	}

	// Slow path: full detection.
	result, err := ParseWith(s, p.options()...)
	if err != nil {
		return ParseResult{}, err
	}

	p.layout = result.Layout
	return result, nil
}

// ParseColumn parses a slice of date strings, auto-detecting the
// format on the first non-empty entry and applying it to the rest.
func (p *Parser) ParseColumn(values []string) ([]time.Time, []error) {
	times := make([]time.Time, len(values))
	errs := make([]error, len(values))

	for i, v := range values {
		if v == "" {
			continue
		}
		result, err := p.Parse(v)
		if err != nil {
			errs[i] = err
			continue
		}
		times[i] = result.Time
	}

	return times, errs
}

// Reset clears the cached layout, forcing re-detection on the next call.
func (p *Parser) Reset() {
	p.layout = nil
}

func (p *Parser) options() []Option {
	// Reconstruct options from the stored config.
	var opts []Option
	if p.cfg.preferDayFirst {
		opts = append(opts, WithPreferDayFirst(true))
	}
	if p.cfg.preferYearFirst {
		opts = append(opts, WithPreferYearFirst(true))
	}
	if p.cfg.preferFuture {
		opts = append(opts, WithPreferFuture(true))
	}
	if p.cfg.strictMode {
		opts = append(opts, WithStrictMode(true))
	}
	if p.cfg.timezone != nil {
		opts = append(opts, WithTimezone(p.cfg.timezone))
	}
	if len(p.cfg.locales) > 0 {
		opts = append(opts, WithLocales(p.cfg.locales...))
	}
	opts = append(opts, WithBaseTime(p.cfg.baseTime))
	return opts
}
