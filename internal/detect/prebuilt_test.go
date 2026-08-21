package detect

import (
	"testing"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
)

// TestPrebuiltDefsHoldsEveryPrebuiltEntry checks the accessor against the trie
// it reports on, so a format added later is covered without anybody adding it
// to a list here.
func TestPrebuiltDefsHoldsEveryPrebuiltEntry(t *testing.T) {
	defs := PrebuiltDefs()
	if len(defs) == 0 {
		t.Fatal("no prebuilt defs")
	}
	in := make(map[*compile.FormatDef]bool, len(defs))
	for _, d := range defs {
		if d == nil {
			t.Fatal("PrebuiltDefs holds a nil")
		}
		if in[d] {
			t.Errorf("%s appears twice", d.Name)
		}
		in[d] = true
	}

	entries := 0
	forEachEntry(t, func(e *formatEntry) {
		if e.def == nil {
			return
		}
		entries++
		if !in[e.def] {
			t.Errorf("%s has a prebuilt def that PrebuiltDefs does not report", e.name)
		}
	})
	if entries != len(defs) {
		t.Errorf("%d entries carry a prebuilt def, PrebuiltDefs reports %d", entries, len(defs))
	}
}

// forEachEntry walks every terminal entry in the global trie.
func forEachEntry(t *testing.T, fn func(*formatEntry)) {
	t.Helper()
	var walk func(*trieNode)
	walk = func(n *trieNode) {
		if n == nil {
			return
		}
		if n.entry != nil {
			fn(n.entry)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(&globalTrie.root)
}

// TestPrebuiltDefsIsACopy checks that a caller cannot reach the package's own
// backing array through the accessor. The pointers in it are shared on purpose;
// the slice around them is not.
func TestPrebuiltDefsIsACopy(t *testing.T) {
	a := PrebuiltDefs()
	if len(a) == 0 {
		t.Fatal("no prebuilt defs")
	}
	want := a[0]
	a[0] = nil
	if got := PrebuiltDefs()[0]; got != want {
		t.Error("writing to the returned slice changed what the package holds")
	}
}

// TestOnlyACanonicalTrieHitReturnsAPrebuiltDef is the safety property the whole
// interning scheme rests on. A caller keys a cache on the Def pointer, so a Def
// that is one of the prebuilt ones has to mean the format was matched
// canonically and its fields are the shared fields.
//
// A wrong answer here is not a slow parse, it is the wrong program: a layout
// describing dashes handed back for an input written with slashes.
func TestOnlyACanonicalTrieHitReturnsAPrebuiltDef(t *testing.T) {
	cfg := Config{Timezone: time.UTC}
	prebuilt := make(map[*compile.FormatDef]bool)
	for _, d := range PrebuiltDefs() {
		prebuilt[d] = true
	}

	for _, s := range []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"2024-03-15 10:30:00",
		"20240315",
		"10:30:00",
	} {
		r, ok := Detect(s, cfg)
		if !ok {
			t.Errorf("%q: no match", s)
			continue
		}
		if !prebuilt[r.Def] {
			t.Errorf("%q: canonical trie hit did not return a prebuilt def", s)
		}
	}

	// Respelled separators, textual months, variable-width numerics, the ISO
	// week family and the ambiguous numeric family all build their own def.
	for _, s := range []string{
		"2024/03/15",
		"2024.03.15",
		"March 15, 2024",
		"15-Mar-2024",
		"3/15/2024",
		"01/02/2024",
		"2024-W11-5",
		"2024-074",
		"Fri Mar 15 10:30:00 2024",
	} {
		r, ok := Detect(s, cfg)
		if !ok {
			t.Errorf("%q: no match", s)
			continue
		}
		if prebuilt[r.Def] {
			t.Errorf("%q: built its own def but returned a prebuilt pointer", s)
		}
	}
}

// FuzzPrebuiltDefMeansCanonical fuzzes the same promise over whatever the
// corpus produces: a Result whose Def is one of the prebuilt pointers has to
// carry that def's own layout, unrespelled, and no ambiguity.
func FuzzPrebuiltDefMeansCanonical(f *testing.F) {
	for _, s := range []string{
		"2024-03-15", "2024/03/15", "2024-03-15T10:30:00Z", "20240315",
		"10:30:00", "March 15, 2024", "01/02/2024", "2024-W11", "",
	} {
		f.Add(s)
	}
	prebuilt := make(map[*compile.FormatDef]bool)
	for _, d := range PrebuiltDefs() {
		prebuilt[d] = true
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, ok := Detect(s, Config{Timezone: time.UTC})
		if !ok || !prebuilt[r.Def] {
			return
		}
		if r.Ambig || r.AmbigProne {
			t.Fatalf("%q: prebuilt def %s reports ambiguity", s, r.Def.Name)
		}
		// The def a caller caches describes its own literals, so the input has
		// to have spelled them that way. Anything else respells, which builds a
		// fresh def and cannot reach this line.
		if len(s) == len(r.Def.GoLayout) {
			for _, fld := range r.Def.Fields {
				if fld.Kind != compile.FLiteral || fld.Len != 1 {
					continue
				}
				off := int(fld.Offset)
				if off >= len(s) || off >= len(r.Def.GoLayout) {
					continue
				}
				if s[off] != r.Def.GoLayout[off] && r.Def.GoLayout[off] != '.' {
					t.Fatalf("%q: prebuilt def %s writes %q at %d, input has %q",
						s, r.Def.Name, r.Def.GoLayout[off], off, s[off])
				}
			}
		}
	})
}
