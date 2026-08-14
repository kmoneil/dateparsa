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
	//
	// Strict mode declines it whenever the format can need a guess, which is
	// not the same as whether the parse that produced it did. Ambiguity is a
	// property of the input, so a cached layout cannot answer it for the next
	// value, and the only way to know is to detect again. A caller who asked
	// to be told about guesses is paying for that on purpose.
	//
	// The cache used to take strict mode away after one success: a Parser
	// seeded with "25/12/2024", unambiguous because 25 cannot be a month, then
	// read "01/02/2024" through the cached layout and returned a time where
	// ParseWith returns an *AmbiguousDateError.
	if p.layout != nil && !(p.cfg.strictMode && p.layout.ambiguityProne) {
		t, err := p.layout.Parse(s)
		if err == nil {
			return ParseResult{
				Time:   t,
				Layout: p.layout,
				// Carried from the layout, not left at false. Detection said
				// this format was a guess; reusing it does not make it certain.
				Ambiguous: p.layout.ambiguous,
				Kind:      KindAbsolute,
			}, nil
		}
	}

	// Slow path: full detection using stored config directly (no option reconstruction).
	result, err := parseWithConfig(s, p.cfg)
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
