// Package dateparsa provides high-performance date parsing for Go.
//
// It detects date formats automatically via a trie-based scanner,
// returns a reusable Layout for zero-overhead repeat parsing,
// and supports both structured formats and natural language expressions.
//
// Basic usage:
//
//	result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
//	fmt.Println(result.Time)
//
//	// Reuse the detected layout for subsequent parses:
//	t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
package dateparsa

import (
	"fmt"
	"math"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/detect"
	"github.com/kmoneil/dateparsa/internal/epoch"
	"github.com/kmoneil/dateparsa/internal/locale"
	"github.com/kmoneil/dateparsa/internal/natural"
)

// ParseResult is returned by Parse and contains both the parsed time
// and the detected layout for reuse.
type ParseResult struct {
	Time      time.Time
	Layout    *Layout
	Ambiguous bool
	Kind      Kind
}

// Parse auto-detects the format and parses the date string.
// Returns a ParseResult containing both the time and the reusable Layout.
// Uses the default configuration (English locale, UTC base time).
func Parse(s string) (ParseResult, error) {
	return parseWithConfig(s, config{timezone: time.UTC})
}

// ParseWith parses using explicit options (locale, base time, preferences).
func ParseWith(s string, opts ...Option) (ParseResult, error) {
	cfg := buildConfig(opts)
	return parseWithConfig(s, cfg)
}

// parseWithConfig is the internal implementation shared by ParseWith and Parser.
func parseWithConfig(input string, cfg config) (ParseResult, error) {
	// One trim, ahead of the whole cascade, so that the trie, the textual
	// detector, epoch and natural language all see the same value. Two of
	// those four tolerated padding already and two refused it, by accident in
	// both directions; see whitespace.go for what is trimmed and what is not.
	//
	// Every error below carries input rather than s, because the caller logs
	// the value they passed. Only the detectors and the executor see the
	// trimmed span, and every field offset is relative to it, which is why
	// Layout.Parse has to trim the same way for the layout to describe the
	// next row.
	s := trimPadding(input)

	localeDatas := localeDataFromConfig(cfg)

	dcfg := detect.Config{
		PreferDayFirst:  cfg.preferDayFirst,
		PreferYearFirst: cfg.preferYearFirst,
		Timezone:        cfg.timezone,
		Locales:         localeDatas,
	}

	result, ok := detect.Detect(s, dcfg)
	if !ok {
		// Try epoch timestamp.
		if er := epoch.Detect(s); er != nil {
			return ParseResult{
				Time:   er.Time,
				Kind:   KindAbsolute,
				Layout: LayoutEpoch,
			}, nil
		}

		// Try natural language.
		baseTime := cfg.baseTime
		if baseTime.IsZero() {
			baseTime = time.Now()
		}
		nlCfg := natural.Config{
			BaseTime:     baseTime,
			PreferFuture: cfg.preferFuture,
			Locales:      localeDatas,
		}
		if nlr := natural.Parse(s, nlCfg); nlr != nil {
			kind := KindRelative
			if nlr.Kind == natural.KindNow {
				kind = KindNow
			}
			// A locale that spells two meanings the same way produces two
			// readings, and this is the one place that can say so. Strict mode
			// refuses with both, exactly as it does for DD/MM against MM/DD;
			// otherwise the guess is reported and not hidden.
			//
			// Neither happened before: this path built a ParseResult with
			// Ambiguous left false, and the strict-mode check below is past the
			// return, so strict mode never saw a natural-language parse at all.
			// Hindi "कल" is both yesterday and tomorrow, and it came back as
			// tomorrow with Ambiguous false and no error.
			if nlr.Ambiguous && cfg.strictMode {
				return ParseResult{}, &AmbiguousDateError{
					Input: input,
					Interpretations: []Interpretation{
						{Time: nlr.Time, Layout: LayoutNaturalLanguage, Label: nlr.Time.Format(nlLabelFormat)},
						{Time: nlr.AltTime, Layout: LayoutNaturalLanguage, Label: nlr.AltTime.Format(nlLabelFormat)},
					},
				}
			}
			return ParseResult{
				Time:      nlr.Time,
				Kind:      kind,
				Layout:    LayoutNaturalLanguage,
				Ambiguous: nlr.Ambiguous,
			}, nil
		}

		return ParseResult{}, &ParseError{Input: input, Message: "no matching format found", Cause: ErrNoMatch}
	}

	if cfg.strictMode && result.Ambig {
		return ParseResult{}, buildAmbiguousError(input, s, cfg, result)
	}

	// A trie format compiled with the default timezone has a Layout built for
	// it at init, and this is where that is spent: no Compile, no allocation,
	// and the same *Layout handed to every caller who parses that format. See
	// interned.go for the gate and for why sharing one is sound.
	var (
		layout *Layout
		t      time.Time
		err    error
	)
	if l := internedLayout(result, cfg); l != nil {
		layout = l
		t, err = l.program.Execute(s)
	} else {
		// The base year is compiled into the program rather than patched onto
		// the result, so that the Layout returned below reproduces this call.
		// Patching here left Parse("10:30:00") and Layout.Parse("10:30:00")
		// disagreeing by two thousand years, and it could not tell a format
		// with no year field from a year field that read 0, so
		// Parse("0000-01-01") returned the current year.
		program, needsBaseYear, cerr := compile.Compile(result.Def, cfg.timezone)
		if cerr != nil {
			return ParseResult{}, &ParseError{Input: input, Message: cerr.Error(), Cause: ErrNoMatch}
		}
		if needsBaseYear {
			by, ok := baseYear(cfg)
			if !ok {
				return ParseResult{}, baseYearError(input, cfg)
			}
			program.BaseYear = by
		}

		// Executed from the stack copy and before the Layout exists, which is
		// the order this had before interning and has to keep. Building the
		// Layout first and running layout.program instead reads the program
		// back out of memory the allocator has just handed over, and it cost
		// Parse_TextualMonth +17.9% and Parse_Miss_Short +11.1% at p=0.000 on
		// linux/arm64. It also allocated a Layout for a parse that then failed.
		t, err = program.Execute(s)
		if err == nil {
			layout = &Layout{
				program:  program,
				goLayout: result.Def.GoLayout,
				label:    result.Def.Name,

				ambiguous:      result.Ambig,
				ambiguityProne: result.AmbigProne,
				trimsPadding:   true,
			}
		}
	}
	if err != nil {
		return ParseResult{}, &ParseError{Input: input, Message: err.Error(), Cause: ErrNoMatch}
	}

	return ParseResult{
		Time:      t,
		Layout:    layout,
		Ambiguous: result.Ambig,
		Kind:      KindAbsolute,
	}, nil
}

// ParseTime is a convenience function that returns only the time.Time.
func ParseTime(s string, opts ...Option) (time.Time, error) {
	result, err := ParseWith(s, opts...)
	if err != nil {
		return time.Time{}, err
	}
	return result.Time, nil
}

// Detect analyzes a date string and returns the detected Layout
// without parsing. Useful when you want to inspect the format
// before committing to parsing.
func Detect(input string, opts ...Option) (*Layout, error) {
	cfg := buildConfig(opts)

	// Same trim as parseWithConfig, for the same reason: this returns a layout
	// a caller reuses, and it has to be the layout Parse would have returned
	// for the same value.
	s := trimPadding(input)

	dcfg := detect.Config{
		PreferDayFirst:  cfg.preferDayFirst,
		PreferYearFirst: cfg.preferYearFirst,
		Timezone:        cfg.timezone,
		Locales:         localeDataFromConfig(cfg),
	}

	result, ok := detect.Detect(s, dcfg)
	if !ok {
		return nil, &ParseError{Input: input, Message: "no matching format found", Cause: ErrNoMatch}
	}

	// The same shared layout parseWithConfig uses, and it has to be the same
	// one: Detect is documented as returning the layout Parse would have
	// returned for this value.
	if l := internedLayout(result, cfg); l != nil {
		return l, nil
	}

	program, needsBaseYear, err := compile.Compile(result.Def, cfg.timezone)
	if err != nil {
		return nil, &ParseError{Input: input, Message: err.Error(), Cause: ErrNoMatch}
	}
	if needsBaseYear {
		by, ok := baseYear(cfg)
		if !ok {
			return nil, baseYearError(input, cfg)
		}
		program.BaseYear = by
	}
	return &Layout{
		program:  program,
		goLayout: result.Def.GoLayout,
		label:    result.Def.Name,

		ambiguous:      result.Ambig,
		ambiguityProne: result.AmbigProne,
		trimsPadding:   true,
	}, nil
}

// nlLabelFormat labels the two readings of an ambiguous natural-language
// phrase. The readings differ by a day, so the date is what distinguishes them
// and the word that produced each does not: "कल" is the label for both.
const nlLabelFormat = "2006-01-02"

// buildAmbiguousError returns the readings strict mode refused to choose
// between, as an *AmbiguousDateError carrying one Interpretation each.
//
// It used to read the input twice itself, once with PreferDayFirst false and
// once with it true, and label the two answers MM/DD/YYYY and DD/MM/YYYY. That
// works only for an input whose ambiguity is the field order and whose format
// consults the preference, and two of the three heuristics ahead of that
// preference can overrule it. detect.Result.Readings is where the pair is built
// now, out of the format already detected; its doc comment has the two shapes
// this got wrong.
//
// What is left here is the caller's half: compile each reading, read the input
// with it, and keep the ones that parse. A reading that does not parse is not
// one of the ones the caller is being asked about, and fewer than two of them
// leaves nothing to choose between, so the ErrAmbiguous ParseError stands in
// exactly where it did before.
//
// There are two readings for most ambiguous inputs and three for a year-first
// one, because "01/02/03" read as 2001-02-03 was chosen against both orders of
// the year-last reading rather than against one of them.
//
// The base year and the timezone come from cfg because the readings are the
// caller's readings. "March 15" has no year field on either reading, and the
// pair is only comparable if both take the same base.
// input is the value as the caller passed it and s is that value trimmed. The
// readings are compiled against s, because their field offsets are relative to
// it, and the error names input, because that is what the caller holds.
func buildAmbiguousError(input, s string, cfg config, result detect.Result) error {
	var interps []Interpretation
	for _, r := range result.Readings() {
		in, parsed, err := interpretation(input, s, cfg, r.Def, r.Label)
		if err != nil {
			return err
		}
		if parsed {
			interps = append(interps, in)
		}
	}

	// One reading is not a choice. "March 00" is the case: the day reading is
	// not a date and only the year one parses, and an *AmbiguousDateError
	// holding it would offer strict mode's caller a value the lenient path
	// refuses outright.
	if len(interps) < 2 {
		return &ParseError{
			Input:   input,
			Message: "ambiguous date could not be interpreted",
			Cause:   ErrAmbiguous,
		}
	}
	return &AmbiguousDateError{
		Input:           input,
		Interpretations: interps,
	}
}

// interpretation compiles def and reads s with it.
//
// ok is false when the format does not read this input, which is not an error:
// a reading that cannot parse the input is not one of the ones the caller is
// being asked to choose between, and the other reading may still be. The error
// return is for the two failures that belong to the call rather than to the
// reading, a program that will not compile and a base year that will not fit.
func interpretation(input, s string, cfg config, def *compile.FormatDef, label string) (Interpretation, bool, error) {
	prog, needsBaseYear, err := compile.Compile(def, cfg.timezone)
	if err != nil {
		return Interpretation{}, false, &ParseError{Input: input, Message: err.Error(), Cause: ErrNoMatch}
	}
	if needsBaseYear {
		by, ok := baseYear(cfg)
		if !ok {
			return Interpretation{}, false, baseYearError(input, cfg)
		}
		prog.BaseYear = by
	}
	t, err := prog.Execute(s)
	if err != nil {
		return Interpretation{}, false, nil
	}
	return Interpretation{
		Time: t,
		// Detected, not compiled, so it trims like the one Parse returns. A
		// caller who takes the reading they wanted out of an
		// *AmbiguousDateError and keeps it has the same layout Parse would
		// have handed them.
		Layout: &Layout{program: prog, label: label, trimsPadding: true},
		Label:  label,
	}, true, nil
}

// baseYear is the year a format carrying no year field takes, for example
// "10:30:00" or "March 15". WithBaseTime sets it; otherwise it is the current
// year. Read at detection time, never on Layout.Parse, and only for the formats
// that actually lack a year: the clock read is about 50ns, which is a third of
// a whole Parse, and calling it unconditionally cost 30% on every format.
//
// It reports whether the year fits Program.BaseYear, which is an int32 for a
// documented size reason: a Layout lands in the 208-byte size class exactly and
// widening this field moves it. time.Time reaches year 292277026596, so the
// conversion can truncate, and truncating turns a base year nobody meant into a
// different base year nobody meant. ParseWith("10:30:00") with such a base came
// back as 219250468-01-01 with a nil error.
//
// Reachable, not hypothetical. flextime's numeric paths built exactly such a time
// out of a JSON number until C20, so a caller resolving a record's relative dates
// against the record's own timestamp could hand one straight back. The threat
// model calls the options trusted, which says nobody hostile chooses them; it does
// not say the conversion is safe.
func baseYear(cfg config) (int32, bool) {
	y := 0
	if cfg.baseTime.IsZero() {
		y = time.Now().Year()
	} else {
		y = cfg.baseTime.Year()
	}
	if y < math.MinInt32 || y > math.MaxInt32 {
		return 0, false
	}
	return int32(y), true
}

// baseYearError is what a base year that cannot be represented returns. Its own
// function because four call sites need it and the message is worth writing once.
func baseYearError(s string, cfg config) error {
	return &ParseError{
		Input:   s,
		Message: fmt.Sprintf("base year %d does not fit the compiled layout", cfg.baseTime.Year()),
		Cause:   ErrNoMatch,
	}
}

// localeDataFromConfig extracts the internal locale data pointers from
// the user-facing Locale values in the config.
func localeDataFromConfig(cfg config) []*locale.Data {
	if len(cfg.locales) == 0 {
		return nil
	}
	datas := make([]*locale.Data, 0, len(cfg.locales))
	for _, l := range cfg.locales {
		if l.data != nil {
			datas = append(datas, l.data)
		}
	}
	return datas
}
