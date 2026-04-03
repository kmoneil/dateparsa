package natural

import (
	"time"

	"github.com/kmoneil/dateparsa/internal/locale"
)

// Config holds options for natural language parsing.
type Config struct {
	BaseTime     time.Time
	PreferFuture bool
	Locales      []*locale.Data
}

// Parse attempts to parse a natural language date expression.
// Returns nil if the input is not a recognized NL expression.
func Parse(s string, cfg Config) *Result {
	// Try English tokenizer first (always available).
	if r := tryParse(Scan(s), cfg); r != nil {
		return r
	}

	// Try locale-specific tokenizers.
	for _, loc := range cfg.Locales {
		tokens := ScanLocale(s, loc)
		if r := tryParse(tokens, cfg); r != nil {
			return r
		}
	}

	return nil
}

func tryParse(tokens []Token, cfg Config) *Result {
	if len(tokens) == 0 {
		return nil
	}

	// Require at least one "meaningful" NL token (not just numbers or unknowns).
	hasMeaningful := false
	for _, t := range tokens {
		switch t.Kind {
		case TokRelWord, TokDirection, TokSelector, TokWeekday,
			TokBoundary, TokNoon, TokMidnight, TokUnit, TokHalf, TokTimeOfDay, TokMonth:
			hasMeaningful = true
		}
	}
	if !hasMeaningful {
		return nil
	}

	return Eval(tokens, cfg.BaseTime, cfg.PreferFuture)
}
