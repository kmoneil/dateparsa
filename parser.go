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
	// Fast path: try the cached layout first, unless the format can need a
	// guess, in which case there is nothing to reuse.
	//
	// An ambiguity-prone format is one detection resolved by looking at the
	// values, and the readings it chooses between produce the same program:
	// the same fields, the same widths, at the same offsets. So the layout
	// parses the next row whichever way that row wanted to be read, and
	// succeeding tells you nothing. Reuse is only sound where the format is a
	// property of the shape.
	//
	// This gate was on strict mode alone, and both halves of that were wrong.
	//
	// The flag: a Parser seeded with "03/15/2024", unambiguous because 15
	// cannot be a month, read "01/02/2024" through the cached layout and
	// reported Ambiguous false where ParseWith reports true. Seeded the other
	// way round it reported true where ParseWith reports false. Eight inputs
	// across the slash, dot and dash forms, in both directions.
	//
	// The instant, which is worse and is why this is not a flag fix: a Parser
	// seeded with "MAY70" read "MAY10" as 2010-05-01 where detection reads the
	// tenth of May 2026, and one seeded with "March 32" read "March 31" as
	// 2031-03-01. Ten inputs, each a successful parse of the wrong day, with
	// Ambiguous false and no error. textualDayIsAGuess did not call those
	// formats prone; it does now, and this gate is what acts on it.
	//
	// Re-detecting costs the cache on exactly the formats where a cached
	// answer cannot be trusted, and on nothing else: every format whose fields
	// are fixed by its shape is unaffected, which is every trie format bar the
	// numeric slash family. Measured in the commit that made this change.
	if p.layout != nil && !p.layout.ambiguityProne {
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
//
// An empty cell is skipped: its time is the zero time.Time and its error is
// nil, which is the same pair a row holding a successfully parsed zero time
// produces. **A caller who needs to tell those apart has to check the input
// slice**, because this signature cannot express the difference. Reading
// times[i] alone will not do it, and neither will errs[i].
//
// The alternative, a sentinel error for an empty cell, would make every caller
// filter an error that is not one. _plans/two-pass-column.md proposes a
// ColumnReport carrying explicit per-row status, which is where this belongs.
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
