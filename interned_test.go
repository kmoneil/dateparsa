package dateparsa

import (
	"sync"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/detect"
)

// freshLayout builds the Layout parseWithConfig would have built for this
// result before interning existed. It is the reference every test in this file
// compares against, so the comparison is against the code that was replaced and
// not against a second copy of the replacement.
func freshLayout(t *testing.T, result detect.Result, cfg config) *Layout {
	t.Helper()
	program, needsBaseYear, err := compile.Compile(result.Def, cfg.timezone)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if needsBaseYear {
		by, ok := baseYear(cfg)
		if !ok {
			t.Fatalf("base year does not fit")
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
	}
}

// sameLayout compares every field of two Layouts, which is what makes this a
// test of the whole value rather than of the parts somebody remembered.
//
// Program is compared with ==, which works because it holds an array, two ints,
// a pointer and three numbers and nothing that is not comparable. A field added
// to it that is not comparable fails this line at build time rather than
// silently narrowing what the test looks at, and that is the point of not
// reaching for reflect here.
func sameLayout(t *testing.T, got, want *Layout, ctx string) {
	t.Helper()
	if got.program != want.program {
		t.Errorf("%s: program differs\n got %+v\nwant %+v", ctx, got.program, want.program)
	}
	if got.goLayout != want.goLayout {
		t.Errorf("%s: goLayout %q, want %q", ctx, got.goLayout, want.goLayout)
	}
	if got.label != want.label {
		t.Errorf("%s: label %q, want %q", ctx, got.label, want.label)
	}
	if got.ambiguous != want.ambiguous {
		t.Errorf("%s: ambiguous %v, want %v", ctx, got.ambiguous, want.ambiguous)
	}
	if got.ambiguityProne != want.ambiguityProne {
		t.Errorf("%s: ambiguityProne %v, want %v", ctx, got.ambiguityProne, want.ambiguityProne)
	}
	if got.trimsPadding != want.trimsPadding {
		t.Errorf("%s: trimsPadding %v, want %v", ctx, got.trimsPadding, want.trimsPadding)
	}
}

// TestInternedLayoutsMatchCompile is the equivalence half of P16: every layout
// built at init is the layout Compile would have built for the same def.
//
// It walks the prebuilt defs rather than a list written here, so a format added
// to the trie later is covered without anybody remembering to add it.
func TestInternedLayoutsMatchCompile(t *testing.T) {
	defs := detect.PrebuiltDefs()
	if len(defs) == 0 {
		t.Fatal("no prebuilt defs; the trie stopped prebuilding them")
	}

	interned := 0
	for _, def := range defs {
		program, needsBaseYear, err := compile.Compile(def, time.UTC)
		if err != nil {
			t.Errorf("%s: Compile: %v", def.Name, err)
			continue
		}
		got := internedLayouts[def]
		if needsBaseYear {
			// A format with no year field reads its base year from the config
			// or from the clock, so there is nothing to prebuild.
			if got != nil {
				t.Errorf("%s: interned despite needing a base year", def.Name)
			}
			continue
		}
		if got == nil {
			t.Errorf("%s: not interned and does not need a base year", def.Name)
			continue
		}
		interned++
		want := &Layout{program: program, goLayout: def.GoLayout, label: def.Name, trimsPadding: true}
		sameLayout(t, got, want, def.Name)
	}
	if interned == 0 {
		t.Fatal("no format was interned, so this test proves nothing")
	}
	if len(internedLayouts) != interned {
		t.Errorf("the table holds %d layouts and %d defs qualify", len(internedLayouts), interned)
	}
	t.Logf("%d of %d prebuilt defs interned", interned, len(defs))
}

// TestInternedLayoutMatchesFreshLayout is the regression half, and it is the
// test that would catch a wrong program reaching a caller.
//
// For every input the coverage list advertises, it detects once, builds the
// layout the old code would have built, and compares it field for field against
// what Parse hands back. An input that is not interned is compared too: the
// answer there has to be the old one exactly, and a gate that widened by
// accident shows up as a difference here.
func TestInternedLayoutMatchesFreshLayout(t *testing.T) {
	checkedInterned, checkedFresh := 0, 0
	for _, tc := range coverageCases {
		cfg := config{timezone: time.UTC}
		if tc.dayFirst {
			cfg.preferDayFirst = true
		}
		s := trimPadding(tc.input)
		dcfg := detect.Config{
			PreferDayFirst:  cfg.preferDayFirst,
			PreferYearFirst: cfg.preferYearFirst,
			Timezone:        cfg.timezone,
		}
		result, ok := detect.Detect(s, dcfg)
		if !ok {
			continue // epoch or natural language; not a detected format
		}

		got, err := parseWithConfig(tc.input, cfg)
		if err != nil {
			t.Errorf("%s (%q): Parse: %v", tc.desc, tc.input, err)
			continue
		}
		want := freshLayout(t, result, cfg)
		sameLayout(t, got.Layout, want, tc.desc+" "+tc.input)

		// And the instant, which is the thing a caller actually reads.
		wantTime, err := want.program.Execute(s)
		if err != nil {
			t.Errorf("%s (%q): reference execute: %v", tc.desc, tc.input, err)
			continue
		}
		if !got.Time.Equal(wantTime) {
			t.Errorf("%s (%q): time %v, want %v", tc.desc, tc.input, got.Time, wantTime)
		}

		if internedLayout(result, cfg) != nil {
			checkedInterned++
		} else {
			checkedFresh++
		}
	}
	if checkedInterned == 0 || checkedFresh == 0 {
		t.Fatalf("interned %d and fresh %d; this test needs both sides to run",
			checkedInterned, checkedFresh)
	}
	t.Logf("%d inputs through the interned path, %d through the compiling path",
		checkedInterned, checkedFresh)
}

// TestInternedLayoutIsShared states the thing that is now observable: two
// parses of the same trie format hand back the same pointer.
//
// It is here so that the sharing is a decision with a test on it rather than an
// implementation detail somebody discovers. If this ever has to stop being
// true, this test is where the reason gets written down.
func TestInternedLayoutIsShared(t *testing.T) {
	a, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse("1999-12-31T23:59:59Z")
	if err != nil {
		t.Fatal(err)
	}
	if a.Layout != b.Layout {
		t.Error("two parses of one trie format returned different layouts")
	}
	c, err := Detect("2001-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if c != a.Layout {
		t.Error("Detect returned a different layout from Parse for the same format")
	}
}

// TestInternedLayoutDeclinedForNonDefaultConfig is the other half of the gate:
// anything the prebuilt layout does not describe gets a fresh one.
func TestInternedLayoutDeclinedForNonDefaultConfig(t *testing.T) {
	shared, err := Parse("2024-03-15T10:30:00")
	if err != nil {
		t.Fatal(err)
	}

	tokyo := time.FixedZone("JST", 9*3600)
	custom, err := ParseWith("2024-03-15T10:30:00", WithTimezone(tokyo))
	if err != nil {
		t.Fatal(err)
	}
	if custom.Layout == shared.Layout {
		t.Fatal("a non-UTC timezone reused the shared layout")
	}
	if _, off := custom.Time.Zone(); off != 9*3600 {
		t.Errorf("offset %d, want %d", off, 9*3600)
	}

	// A respelled separator builds its own def, so it must not be interned:
	// the shared layout describes dashes and this input writes slashes.
	slashed, err := Parse("2024/03/15")
	if err != nil {
		t.Fatal(err)
	}
	dashed, err := Parse("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if slashed.Layout == dashed.Layout {
		t.Fatal("a respelled layout reused the shared one")
	}
	if got, ok := slashed.Layout.GoLayout(); !ok || got != "2006/01/02" {
		t.Errorf("respelled GoLayout %q ok=%v, want %q", got, ok, "2006/01/02")
	}
}

// TestInternedLayoutSurvivesConcurrentUse runs the shared layout from many
// goroutines at once. Under -race this is the check that nothing on the shared
// path writes, which is the promise the whole scheme rests on.
func TestInternedLayoutSurvivesConcurrentUse(t *testing.T) {
	const goroutines, iterations = 8, 500
	want, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan string, goroutines)
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for range iterations {
				r, err := Parse("2024-03-15T10:30:00Z")
				if err != nil {
					errs <- "parse: " + err.Error()
					return
				}
				if r.Layout != want.Layout {
					errs <- "layout pointer changed under concurrent use"
					return
				}
				if !r.Time.Equal(want.Time) {
					errs <- "time changed under concurrent use"
					return
				}
				// Reuse it directly too, which is what a caller holding one does.
				got, err := want.Layout.Parse("2020-01-02T03:04:05Z")
				if err != nil {
					errs <- "reuse: " + err.Error()
					return
				}
				if got.Year() != 2020 {
					errs <- "reuse returned the wrong year"
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

// TestInternedLayoutHasNoWritableSurface is the immutability claim stated as a
// test rather than left to the type.
//
// Layout has no exported fields, so a caller cannot write to a shared one, and
// this asserts that by reflection-free means: every method on *Layout that a
// caller can reach is called on the shared instance, and the instance is
// compared against a copy taken beforehand.
func TestInternedLayoutHasNoWritableSurface(t *testing.T) {
	r, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	l := r.Layout
	before := *l

	_ = l.String()
	_, _ = l.GoLayout()
	_ = l.Reusable()
	if _, err := l.Parse("2020-01-02T03:04:05Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ParseBytes([]byte("2020-01-02T03:04:05Z")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Parse("nonsense"); err == nil {
		t.Fatal("expected a refusal")
	}

	after := *l
	if before != after {
		t.Errorf("a shared Layout changed under its own methods\nbefore %+v\nafter  %+v", before, after)
	}
}

// FuzzInternedLayoutMatchesFresh fuzzes the equivalence claim directly: for any
// input that detects, the Layout Parse hands back has to be the Layout the
// compiling path would have built, field for field, and the instant has to
// match.
//
// The unit tests above walk the trie and the coverage list, which are both
// lists somebody wrote. This walks whatever the corpus produces, which is where
// a gate that admits one input too many shows up. It is deliberately not
// restricted to the interned path: an input that takes the compiling path is
// compared too, so a FormatID leaking onto a result that should not carry one
// fails here rather than silently returning a program for a different format.
func FuzzInternedLayoutMatchesFresh(f *testing.F) {
	for _, tc := range coverageCases {
		f.Add(tc.input)
	}
	for _, s := range []string{
		"2024-03-15", "2024/03/15", "2024.03.15",
		"2024-03-15T10:30:00Z", "2024-03-15T10:30:00+05:30",
		"10:30:00", "10:30", "20240315", "0000-01-01",
		"2024-03-15 10:30:00 UTC", "March 15, 2024", "",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		cfg := config{timezone: time.UTC}
		s := trimPadding(input)
		result, ok := detect.Detect(s, detect.Config{Timezone: cfg.timezone})
		if !ok {
			return // epoch, natural language, or no match: nothing to compare
		}

		program, needsBaseYear, err := compile.Compile(result.Def, cfg.timezone)
		if err != nil {
			return // the compiling path refuses it too; not this test's subject
		}
		if needsBaseYear {
			by, ok := baseYear(cfg)
			if !ok {
				return
			}
			program.BaseYear = by
		}
		wantTime, wantErr := program.Execute(s)

		got, gotErr := parseWithConfig(input, cfg)
		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("%q: reference err=%v, Parse err=%v", input, wantErr, gotErr)
		}
		if wantErr != nil {
			return
		}
		if !got.Time.Equal(wantTime) {
			t.Fatalf("%q: Parse %v, reference %v", input, got.Time, wantTime)
		}

		want := &Layout{
			program:  program,
			goLayout: result.Def.GoLayout,
			label:    result.Def.Name,

			ambiguous:      result.Ambig,
			ambiguityProne: result.AmbigProne,
			trimsPadding:   true,
		}
		if *got.Layout != *want {
			t.Fatalf("%q: layout differs\n got %+v\nwant %+v", input, *got.Layout, *want)
		}

		// And the layout has to re-parse its own input to the same instant,
		// which is the property a caller who keeps it depends on.
		again, err := got.Layout.Parse(input)
		if err != nil {
			t.Fatalf("%q: the returned layout refuses its own input: %v", input, err)
		}
		if !again.Equal(wantTime) {
			t.Fatalf("%q: reuse %v, first parse %v", input, again, wantTime)
		}
	})
}
