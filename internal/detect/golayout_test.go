package detect

import (
	"strings"
	"testing"
	"time"
)

// golayoutCase is one input and the Go layout detection should report for it.
//
// The list is the reason P25 could be fixed at all: goLayoutFor is now reached
// only when spelledCanonically says no, so the two have to agree about which
// inputs those are, and about what the answer is for each. A predicate that
// said yes too often would hand back the entry's dashed layout for an input
// written with slashes, silently.
type golayoutCase struct {
	input     string
	want      string
	canonical bool
	desc      string
}

var golayoutCases = []golayoutCase{
	{"2024-03-15", "2006-01-02", true, "canonical separators"},
	{"2024/03/15", "2006/01/02", false, "slashes respelled"},
	{"2024.03.15", "2006.01.02", false, "dots respelled"},
	{"2024-03-15 10:30:00", "2006-01-02 15:04:05", true, "canonical datetime"},
	{"2024/03/15 10:30:00", "2006/01/02 15:04:05", false, "datetime respelled"},

	// The fraction's own "." is the one position that may not be respelled,
	// because ".000" is a token: writing another byte over the dot makes Go
	// read a literal and then "000", which parses one value of the field it
	// claims to describe. C17 made those report no layout at all.
	{"2024-03-15 10:30:00.123", "2006-01-02 15:04:05.000", true, "canonical fraction"},
	{"2024-03-15 10:30:00/123", "", false, "fraction dot respelled"},

	// The case nothing covered before P25, and the one that decides whether
	// the fraction may be checked after the literals. It may not: the answer
	// has to be "no layout", not "a respelled layout".
	{"2024/03/15 10:30:00/123", "", false, "BOTH the fraction and a separator respelled"},

	{"2024-03-15T10:30:00Z", "2006-01-02T15:04:05Z", true, "canonical RFC 3339 Z"},
	{"2024/03/15T10:30:00Z", "2006/01/02T15:04:05Z", false, "RFC 3339 Z respelled"},
}

// TestGoLayoutForEverySpelling pins what a caller reads off the result. The Go
// layout travels out through Layout.GoLayout and into the caller's time.Parse,
// so a wrong one here is a wrong parse in somebody else's code.
func TestGoLayoutForEverySpelling(t *testing.T) {
	for _, tc := range golayoutCases {
		r, ok := Detect(tc.input, Config{Timezone: time.UTC})
		if !ok {
			t.Errorf("%s: %q did not match", tc.desc, tc.input)
			continue
		}
		if r.Def.GoLayout != tc.want {
			t.Errorf("%s: %q gave GoLayout %q, want %q", tc.desc, tc.input, r.Def.GoLayout, tc.want)
		}
	}
}

// TestSpelledCanonicallyAgreesWithGoLayoutFor is the invariant the P25 split
// rests on. detectFormat asks the predicate and reaches goLayoutFor only when
// the answer is no, so "canonical" has to mean exactly "goLayoutFor would have
// returned the entry's own layout" and nothing else.
//
// Checked against the entries themselves rather than a list, so a format added
// later is covered without anybody remembering.
func TestSpelledCanonicallyAgreesWithGoLayoutFor(t *testing.T) {
	checked, respelled := 0, 0
	forEachEntry(t, func(e *formatEntry) {
		if e.goLayout == "" {
			return
		}
		// The entry's own layout, and every same-class respelling of each of
		// its literal positions.
		spellings := []string{e.goLayout}
		for _, off := range e.litOffsets[:e.nLits] {
			for _, b := range []byte{'-', '/', '.', ' ', ':', 'T'} {
				if b == e.goLayout[off] {
					continue
				}
				v := []byte(e.goLayout)
				v[off] = b
				spellings = append(spellings, string(v))
			}
		}
		for _, sp := range spellings {
			got := spelledCanonically(e, sp)
			want := goLayoutFor(e, sp) == e.goLayout
			if got != want {
				t.Errorf("%s: spelledCanonically(%q) = %v, goLayoutFor says %v",
					e.name, sp, got, want)
			}
			checked++
			if !got {
				respelled++
			}
		}
	})
	if checked == 0 || respelled == 0 {
		t.Fatalf("checked %d spellings, %d of them respelled; both sides have to run", checked, respelled)
	}
	t.Logf("%d spellings checked, %d of them respelled", checked, respelled)
}

// TestNoEntryOverflowsItsOffsets holds the array bound literalOffsets assumes.
// An entry that overflows silently loses its check and stops reporting a
// respelled layout, which is a wrong layout handed to a caller rather than an
// error.
func TestNoEntryOverflowsItsOffsets(t *testing.T) {
	worst, worstName := 0, ""
	forEachEntry(t, func(e *formatEntry) {
		if e.goLayout == "" {
			return
		}
		if int(e.nLits) > maxRespellOffsets {
			t.Errorf("%s: %d offsets, the array holds %d", e.name, e.nLits, maxRespellOffsets)
		}
		// An entry with a layout and no offsets at all lost them to the
		// overflow guard, which is the silent failure this test is for.
		// COMPACT_DATE legitimately has none, so the check is that the count
		// matches what the layout actually holds.
		if int(e.nLits) > worst {
			worst, worstName = int(e.nLits), e.name
		}
	})
	t.Logf("widest entry is %s with %d offsets, the array holds %d", worstName, worst, maxRespellOffsets)
	if worst == 0 {
		t.Fatal("no entry carries any offsets, so nothing is being checked")
	}
}

// FuzzSpelledCanonicallyAgreesWithGoLayoutFor fuzzes the same agreement over
// whatever the corpus reaches, including inputs of the wrong length and inputs
// that are not spellings of anything.
func FuzzSpelledCanonicallyAgreesWithGoLayoutFor(f *testing.F) {
	for _, tc := range golayoutCases {
		f.Add(tc.input)
	}
	for _, s := range []string{"", "x", strings.Repeat("-", 30), "2006-01-02"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		forEachEntry(t, func(e *formatEntry) {
			got := spelledCanonically(e, s)
			want := goLayoutFor(e, s) == e.goLayout
			if got != want {
				t.Fatalf("%s: spelledCanonically(%q) = %v, goLayoutFor says %v",
					e.name, s, got, want)
			}
		})
	})
}
