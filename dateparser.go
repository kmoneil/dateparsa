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
func parseWithConfig(s string, cfg config) (ParseResult, error) {
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
					Input: s,
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

		return ParseResult{}, &ParseError{Input: s, Message: "no matching format found", Cause: ErrNoMatch}
	}

	if cfg.strictMode && result.Ambig {
		return ParseResult{}, buildAmbiguousError(s, cfg)
	}

	// The base year is compiled into the program rather than patched onto the
	// result, so that the Layout returned below reproduces this call. Patching
	// here left Parse("10:30:00") and Layout.Parse("10:30:00") disagreeing by
	// two thousand years, and it could not tell a format with no year field
	// from a year field that read 0, so Parse("0000-01-01") returned the
	// current year.
	program, needsBaseYear, err := compile.Compile(result.Def, cfg.timezone)
	if err != nil {
		return ParseResult{}, &ParseError{Input: s, Message: err.Error(), Cause: ErrNoMatch}
	}
	if needsBaseYear {
		program.BaseYear = int32(baseYear(cfg))
	}
	t, err := program.Execute(s)
	if err != nil {
		return ParseResult{}, &ParseError{Input: s, Message: err.Error(), Cause: ErrNoMatch}
	}

	layout := &Layout{
		program:  program,
		goLayout: result.Def.GoLayout,
		label:    result.Def.Name,

		ambiguous:      result.Ambig,
		ambiguityProne: result.AmbigProne,
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
func Detect(s string, opts ...Option) (*Layout, error) {
	cfg := buildConfig(opts)

	dcfg := detect.Config{
		PreferDayFirst:  cfg.preferDayFirst,
		PreferYearFirst: cfg.preferYearFirst,
		Timezone:        cfg.timezone,
		Locales:         localeDataFromConfig(cfg),
	}

	result, ok := detect.Detect(s, dcfg)
	if !ok {
		return nil, &ParseError{Input: s, Message: "no matching format found", Cause: ErrNoMatch}
	}

	program, needsBaseYear, err := compile.Compile(result.Def, cfg.timezone)
	if err != nil {
		return nil, &ParseError{Input: s, Message: err.Error(), Cause: ErrNoMatch}
	}
	if needsBaseYear {
		program.BaseYear = int32(baseYear(cfg))
	}
	return &Layout{
		program:  program,
		goLayout: result.Def.GoLayout,
		label:    result.Def.Name,

		ambiguous:      result.Ambig,
		ambiguityProne: result.AmbigProne,
	}, nil
}

// nlLabelFormat labels the two readings of an ambiguous natural-language
// phrase. The readings differ by a day, so the date is what distinguishes them
// and the word that produced each does not: "कल" is the label for both.
const nlLabelFormat = "2006-01-02"

func buildAmbiguousError(s string, cfg config) error {
	// Build both interpretations (MDY and DMY).
	var interps []Interpretation

	// MDY interpretation.
	mdyDcfg := detect.Config{PreferDayFirst: false, Timezone: cfg.timezone}
	if mdyResult, ok := detect.Detect(s, mdyDcfg); ok {
		prog, needsBaseYear, cerr := compile.Compile(mdyResult.Def, cfg.timezone)
		if cerr != nil {
			return &ParseError{Input: s, Message: cerr.Error(), Cause: ErrNoMatch}
		}
		if needsBaseYear {
			prog.BaseYear = int32(baseYear(cfg))
		}
		t, err := prog.Execute(s)
		if err == nil {
			interps = append(interps, Interpretation{
				Time:   t,
				Layout: &Layout{program: prog, label: "MM/DD/YYYY"},
				Label:  "MM/DD/YYYY",
			})
		}
	}

	// DMY interpretation.
	dmyDcfg := detect.Config{PreferDayFirst: true, Timezone: cfg.timezone}
	if dmyResult, ok := detect.Detect(s, dmyDcfg); ok {
		prog, needsBaseYear, cerr := compile.Compile(dmyResult.Def, cfg.timezone)
		if cerr != nil {
			return &ParseError{Input: s, Message: cerr.Error(), Cause: ErrNoMatch}
		}
		if needsBaseYear {
			prog.BaseYear = int32(baseYear(cfg))
		}
		t, err := prog.Execute(s)
		if err == nil {
			interps = append(interps, Interpretation{
				Time:   t,
				Layout: &Layout{program: prog, label: "DD/MM/YYYY"},
				Label:  "DD/MM/YYYY",
			})
		}
	}

	if len(interps) == 0 {
		return &ParseError{
			Input:   s,
			Message: "ambiguous date could not be interpreted",
			Cause:   ErrAmbiguous,
		}
	}
	return &AmbiguousDateError{
		Input:           s,
		Interpretations: interps,
	}
}

// baseYear is the year a format carrying no year field takes, for example
// "10:30:00" or "March 15". WithBaseTime sets it; otherwise it is the current
// year. Read at detection time, never on Layout.Parse, and only for the formats
// that actually lack a year: the clock read is about 50ns, which is a third of
// a whole Parse, and calling it unconditionally cost 30% on every format.
func baseYear(cfg config) int {
	if cfg.baseTime.IsZero() {
		return time.Now().Year()
	}
	return cfg.baseTime.Year()
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
