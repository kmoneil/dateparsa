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
	return ParseWith(s)
}

// ParseWith parses using explicit options (locale, base time, preferences).
func ParseWith(s string, opts ...Option) (ParseResult, error) {
	cfg := buildConfig(opts)

	dcfg := detect.Config{
		PreferDayFirst:  cfg.preferDayFirst,
		PreferYearFirst: cfg.preferYearFirst,
		Timezone:        cfg.timezone,
	}

	result := detect.Detect(s, dcfg)
	if result == nil {
		// Try epoch timestamp.
		if er := epoch.Detect(s); er != nil {
			return ParseResult{
				Time: er.Time,
				Kind: KindAbsolute,
			}, nil
		}

		// Try natural language.
		nlCfg := natural.Config{
			BaseTime:     cfg.baseTime,
			PreferFuture: cfg.preferFuture,
		}
		if nlr := natural.Parse(s, nlCfg); nlr != nil {
			kind := KindRelative
			if nlr.Kind == natural.KindNow {
				kind = KindNow
			}
			return ParseResult{
				Time: nlr.Time,
				Kind: kind,
			}, nil
		}

		return ParseResult{}, &ParseError{Input: s, Message: "no matching format found"}
	}

	if cfg.strictMode && result.Ambig {
		return ParseResult{}, buildAmbiguousError(s, result, cfg)
	}

	program := compile.Compile(result.Def, cfg.timezone)
	t, err := program.Execute(s)
	if err != nil {
		return ParseResult{}, &ParseError{Input: s, Message: err.Error()}
	}

	layout := &Layout{
		program:  program,
		goLayout: result.Def.GoLayout,
		label:    result.Def.Name,
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
	}

	result := detect.Detect(s, dcfg)
	if result == nil {
		return nil, &ParseError{Input: s, Message: "no matching format found"}
	}

	program := compile.Compile(result.Def, cfg.timezone)
	return &Layout{
		program:  program,
		goLayout: result.Def.GoLayout,
		label:    result.Def.Name,
	}, nil
}

func buildAmbiguousError(s string, result *detect.Result, cfg config) *AmbiguousDateError {
	// Build both interpretations (MDY and DMY).
	var interps []Interpretation

	// MDY interpretation.
	mdyCfg := cfg
	mdyCfg.preferDayFirst = false
	mdyDcfg := detect.Config{PreferDayFirst: false, Timezone: cfg.timezone}
	mdyResult := detect.Detect(s, mdyDcfg)
	if mdyResult != nil {
		prog := compile.Compile(mdyResult.Def, cfg.timezone)
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
	dmyResult := detect.Detect(s, dmyDcfg)
	if dmyResult != nil {
		prog := compile.Compile(dmyResult.Def, cfg.timezone)
		t, err := prog.Execute(s)
		if err == nil {
			interps = append(interps, Interpretation{
				Time:   t,
				Layout: &Layout{program: prog, label: "DD/MM/YYYY"},
				Label:  "DD/MM/YYYY",
			})
		}
	}

	return &AmbiguousDateError{
		Input:           s,
		Interpretations: interps,
	}
}
