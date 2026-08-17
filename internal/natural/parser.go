package natural

import (
	"strings"
	"time"

	"github.com/kmoneil/dateparsa/internal/locale"
)

// Config holds options for natural language parsing.
type Config struct {
	BaseTime     time.Time
	PreferFuture bool
	Locales      []*locale.Data
}

// MaxInputLen bounds the input this package will tokenise.
//
// It is the only bound on the path, and one check here covers every cost on it,
// which is why it is here and not a token counter inside the two scanners. Both of
// them lower a copy of the whole input before tokenising, and ScanLocale does it
// once per configured locale, so a counter would bound the token slice and leave the
// copies.
//
// The amplification it exists to stop was measured, not estimated. Token is 104
// bytes and Scan appends one per whitespace-separated word, and Go grows a large
// slice by about 1.25x, so the intermediate arrays come to roughly five times the
// final one. On linux/arm64:
//
//	1 MiB of words   135 MB peak heap, 281 MB allocated, 190 ms
//	4 MiB of words   665 MB peak heap, 1.35 GB allocated
//	16 x 1 MiB       1.94 GiB peak, concurrently
//	100 KB, 20 locales               348 MB allocated
//
// SECURITY.md said this path had "no amplification" and that "a 10MB string of words
// costs roughly 10MB of work". It cost about 1.3 GB, and that sentence was what a
// caller would have sized their own input cap from.
//
// 512 rather than something tighter because a compound relative expression is the one
// legitimately long input: "1 day and " repeated is a valid parse, and 512 bytes
// admits about fifty terms. Nothing shorter is refused that used to parse. The path
// is reached only after structured and epoch detection have both failed, so no date
// format is affected by it at all.
const MaxInputLen = 512

// Parse attempts to parse a natural language date expression.
// Returns nil if the input is not a recognized NL expression.
func Parse(s string, cfg Config) *Result {
	// Checked before anything is lowered or tokenised, so an over-long input costs
	// one comparison rather than a copy per locale. See MaxInputLen.
	if len(s) > MaxInputLen {
		return nil
	}

	// Try English tokenizer first (always available).
	if r := tryParse(Scan(s), cfg); r != nil {
		return r
	}

	// Try locale-specific tokenizers.
	//
	// Nothing below runs without a locale, and the lowering must not either:
	// hoisting it above this check cost strings.ToLower on every no-locale
	// miss, which is one allocation for "N/A" and +33% on Parse_Miss_Short.
	// That benchmark is the shape that has punished two earlier reorderings
	// around here.
	if len(cfg.Locales) == 0 {
		return nil
	}

	// Lowering and accent folding are functions of the input and not of the
	// locale, so they happen once here rather than once inside each scan. Three
	// configured locales meant three strings.ToLower and three foldAccents of
	// the same bytes, and foldAccents reads every rune of the input before it
	// can say that nothing folds.
	lowered := foldAccents(strings.ToLower(s))
	for _, loc := range cfg.Locales {
		tokens := scanLowered(lowered, loc)
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
