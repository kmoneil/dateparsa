package dateparsa

import (
	"testing"
	"time"
)

// TestParseZeroAllocForPrebuiltFormats holds an invariant P16 created and
// nothing guarded: a cold Parse of a trie format, with the default config,
// allocates nothing at all.
//
// It used to allocate once, 208 bytes, for the Layout it built. Interning
// removed that, and removing it is 60% of what Detect_Only costs. Nothing
// asserted it afterwards, so the next change to the gate in interned.go, or to
// what parseWithConfig does with the result, could put the allocation back and
// every test in the tree would still pass. TestLayoutParseZeroAlloc covers the
// hot path and says nothing about this one.
//
// The list is walked from the coverage cases rather than written here, so a
// format added later is covered without anybody remembering, and the count
// assertions at the end are what stop the test going quiet if the gate narrows
// to nothing.
func TestParseZeroAllocForPrebuiltFormats(t *testing.T) {
	prebuilt, compiled := 0, 0
	for _, tc := range coverageCases {
		if tc.dayFirst {
			continue // a preference is not the default config
		}
		input := tc.input
		r, err := Parse(input)
		if err != nil {
			continue // epoch, natural language and the ambiguous families
		}
		shared := false
		for _, l := range internedLayouts {
			if l == r.Layout {
				shared = true
				break
			}
		}
		allocs := testing.AllocsPerRun(200, func() {
			_, _ = Parse(input)
		})
		if shared {
			prebuilt++
			if allocs != 0 {
				t.Errorf("%s: Parse(%q) allocates %v times and its layout is prebuilt",
					tc.desc, input, allocs)
			}
			continue
		}
		compiled++
	}
	if prebuilt == 0 {
		t.Fatal("no input reached a prebuilt layout, so this test asserts nothing")
	}
	if compiled == 0 {
		t.Fatal("every input reached a prebuilt layout; the compiling path is untested here")
	}
	t.Logf("%d inputs parse with no allocation, %d compile a layout", prebuilt, compiled)
}

// TestDetectZeroAllocForPrebuiltFormats is the same promise for Detect, which
// is the entry point a caller uses to inspect a format before committing to it.
func TestDetectZeroAllocForPrebuiltFormats(t *testing.T) {
	for _, input := range []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00+05:30",
		"2024-03-15 10:30:00",
		"20240315",
	} {
		if _, err := Detect(input); err != nil {
			t.Errorf("Detect(%q): %v", input, err)
			continue
		}
		if allocs := testing.AllocsPerRun(200, func() { _, _ = Detect(input) }); allocs != 0 {
			t.Errorf("Detect(%q) allocates %v times", input, allocs)
		}
	}
}

// TestParseAllocatesWhenItMustSays what the gate costs, so that a change which
// silently widens it is visible as this test going quiet rather than as a
// number nobody watches.
//
// A non-UTC timezone and a format with no year field both compile per call, by
// design: the program differs. If either of these ever reaches zero, the gate
// has widened and the equivalence tests in interned_test.go are what should be
// consulted before celebrating.
func TestParseAllocatesWhenItMust(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	if allocs := testing.AllocsPerRun(200, func() {
		_, _ = ParseWith("2024-03-15T10:30:00", WithTimezone(tokyo))
	}); allocs == 0 {
		t.Error("a non-UTC timezone reached a prebuilt layout; the gate has widened")
	}
	if allocs := testing.AllocsPerRun(200, func() { _, _ = Parse("10:30:00") }); allocs == 0 {
		t.Error("a format with no year field reached a prebuilt layout; it cannot, its base year comes from the clock")
	}
}
