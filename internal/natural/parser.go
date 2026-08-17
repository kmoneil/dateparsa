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
		if r := parseBothReadings(tokens, cfg); r != nil {
			return r
		}
	}

	return nil
}

// parseBothReadings evaluates the tokens, and when one of them is a phrase its
// locale lists under two meanings of the same kind, evaluates the other reading
// too and reports the pair.
//
// Only the locale scanner can produce such a token, so the English path above
// does not go through here. See Token.Alt and mergeSameKindDuplicates.
//
// Two readings that land on the same instant are not an ambiguity, and neither
// is one the grammar refuses: "बीता कल" is yesterday under both readings of
// कल, because the qualifier is a longer phrase and matches ahead of the bare
// word. Checking rather than assuming is what keeps this from reporting
// ambiguity on input that has none.
func parseBothReadings(tokens []Token, cfg Config) *Result {
	r := tryParse(tokens, cfg)
	if r == nil {
		return nil
	}

	swapped, ok := swapAlternates(tokens)
	if !ok {
		return r
	}
	alt := tryParse(swapped, cfg)
	if alt == nil || alt.Time.Equal(r.Time) {
		return r
	}

	r.Ambiguous = true
	r.AltTime = alt.Time
	return r
}

// swapAlternates returns the token stream with every two-reading phrase read
// the other way, and reports whether there was one. The copy is made only when
// there is.
func swapAlternates(tokens []Token) ([]Token, bool) {
	found := false
	for i := range tokens {
		if tokens[i].Alt != nil {
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}

	swapped := make([]Token, len(tokens))
	copy(swapped, tokens)
	for i := range swapped {
		if swapped[i].Alt == nil {
			continue
		}
		// Keep where it matched and what matched; take only the meaning.
		alt := *swapped[i].Alt
		alt.Pos, alt.Raw, alt.Alt = swapped[i].Pos, swapped[i].Raw, nil
		swapped[i] = alt
	}
	return swapped, true
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
