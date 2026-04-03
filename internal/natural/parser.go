package natural

import "time"

// Config holds options for natural language parsing.
type Config struct {
	BaseTime     time.Time
	PreferFuture bool
}

// Parse attempts to parse a natural language date expression.
// Returns nil if the input is not a recognized NL expression.
func Parse(s string, cfg Config) *Result {
	tokens := Scan(s)
	if len(tokens) == 0 {
		return nil
	}

	// Require at least one "meaningful" NL token (not just numbers or unknowns).
	// This prevents false-positives on structured date strings.
	hasMeaningful := false
	for _, t := range tokens {
		switch t.Kind {
		case TokRelWord, TokDirection, TokSelector, TokWeekday,
			TokBoundary, TokNoon, TokMidnight, TokUnit:
			hasMeaningful = true
		}
	}
	if !hasMeaningful {
		return nil
	}

	return Eval(tokens, cfg.BaseTime, cfg.PreferFuture)
}
