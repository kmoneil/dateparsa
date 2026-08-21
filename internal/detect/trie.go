package detect

import "github.com/kmoneil/dateparsa/internal/compile"

// trieNode is a node in the format signature trie.
// Children are indexed by CharClass (0..5), keeping the structure compact.
type trieNode struct {
	children [numClasses]*trieNode
	entry    *formatEntry // Non-nil if this node is a terminal (matches a format)
}

// trie is the root of the format signature trie.
type trie struct {
	root trieNode
}

// insert adds a format entry to the trie keyed by its signature.
func (t *trie) insert(e *formatEntry) {
	node := &t.root
	for _, cc := range e.sig {
		child := node.children[cc]
		if child == nil {
			child = &trieNode{}
			node.children[cc] = child
		}
		node = child
	}
	node.entry = e
}

// lookup walks the trie with the given signature and returns the matching
// format entry, or nil if no match.
func (t *trie) lookup(sig *Signature) *formatEntry {
	node := &t.root
	for i := 0; i < sig.len; i++ {
		cc := sig.buf[i]
		child := node.children[cc]
		if child == nil {
			return nil
		}
		node = child
	}
	return node.entry
}

// prebuiltDefs holds every FormatDef buildTrie prebuilt, which are the defs a
// Result carries by pointer rather than building per call.
var prebuiltDefs []*compile.FormatDef

// PrebuiltDefs returns the FormatDefs the trie hands back by pointer.
//
// It exists so the caller that owns the compiled representation can build one
// of those per def at init and find it again by the pointer a Result carries,
// rather than compiling the same def on every parse. A def in this slice is
// immutable and shared: it is the same pointer every Result naming that format
// holds, and a detector that builds its own def is therefore never one of
// these. That identity is the whole guarantee, so the caller needs no other
// key and this package needs no field to carry one.
//
// The slice is freshly made per call so a caller cannot reach the package's own
// backing array. The pointers in it are the real ones, deliberately.
func PrebuiltDefs() []*compile.FormatDef {
	out := make([]*compile.FormatDef, len(prebuiltDefs))
	copy(out, prebuiltDefs)
	return out
}

// build constructs the trie from all known format definitions.
func buildTrie() *trie {
	t := &trie{}
	for _, formats := range [][]formatEntry{phase1Formats(), phase2Formats()} {
		for i := range formats {
			if len(formats[i].sig) > 0 {
				stampLiteralClasses(&formats[i])
				formats[i].litOffsets, formats[i].nLits, formats[i].hasFrac = literalOffsets(formats[i].goLayout)
				// Pre-build the FormatDef so Detect doesn't allocate one per call.
				if !formats[i].ambig && len(formats[i].fields) > 0 {
					formats[i].def = &compile.FormatDef{
						Name:     formats[i].name,
						GoLayout: formats[i].goLayout,
						Fields:   formats[i].fields,
					}
					prebuiltDefs = append(prebuiltDefs, formats[i].def)
				}
				t.insert(&formats[i])
			}
		}
	}
	return t
}

// litClassOf maps a signature position's character class to the class the
// compiled literal at that position accepts.
//
// CDigit has no entry worth making: a literal never sits at a digit position,
// and mapping it to ClassAny would leave the format refusing the very input it
// matched, since ClassAny is the one class that excludes digits.
// TestTrieLiteralsCarryTheirClass is what holds that to be true.
var litClassOf = [numClasses]compile.LitClass{
	CLetter:  compile.ClassLetter,
	CSep:     compile.ClassSep,
	CSpace:   compile.ClassSpace,
	CColon:   compile.ClassColon,
	CSpecial: compile.ClassSpecial,
}

// literalOffsets returns the positions of the bytes a Go layout writes
// verbatim, which are the ones an input may spell differently.
//
// Asked of the layout parser rather than guessed, because the punctuation that
// looks like a separator is not always one: the "-" of "-07:00" and the "Z" of
// "Z07:00" are the zone token's own bytes and an input writing "+" there is
// writing a value, not a different separator. ParseGoLayout emits one FLiteral
// per verbatim byte and folds the rest into fields, so the answer falls out of
// it.
//
// **The fraction's own "." is first in the array when the layout has one**, and
// hasFrac says so. That position is not respelled: ".000" is a token, and
// writing the input's "/" over its dot makes Go read a literal "/" followed by
// a literal "000", which parses "00:00:00/000" and refuses "00:00:00/010". A
// layout that takes one value of the field it describes is worse than no
// layout, so the entry reports none for an input spelled that way.
//
// Putting it first is what lets goLayoutFor ask one question instead of two.
// The order is load-bearing rather than tidy: an input that respells the
// fraction's dot AND a separator has to report no layout rather than a
// respelled one, so the fraction has to be the mismatch that is found first.
// TestBothRespelledGivesNoLayout is what holds that.
//
// A layout the parser refuses gets no offsets and no check, which leaves that
// entry exactly as it was, and so does one with more literals than the array
// holds. Nine covers every entry in the tree with headroom: the widest is
// RFC3339_NANO, seven literals and a fraction.
// TestNoEntryOverflowsItsOffsets checks that rather than trusting this comment.
func literalOffsets(goLayout string) (offs [maxRespellOffsets]uint8, n uint8, hasFrac bool) {
	if goLayout == "" || len(goLayout) > 255 {
		return offs, 0, false
	}
	def, err := compile.ParseGoLayout(goLayout)
	if err != nil {
		return offs, 0, false
	}
	fracAt := -1
	for _, f := range def.Fields {
		if f.Kind == compile.FFracSec && f.Offset > 0 {
			fracAt = int(f.Offset) - 1
		}
	}
	// Leave room at the front for the fraction, and drop it afterwards if the
	// layout has none, so the array is built in one pass either way.
	n = 1
	for _, f := range def.Fields {
		if f.Kind != compile.FLiteral {
			continue
		}
		if int(f.Offset) == fracAt {
			offs[0] = uint8(f.Offset)
			hasFrac = true
			continue
		}
		if int(n) == len(offs) {
			return [maxRespellOffsets]uint8{}, 0, false
		}
		offs[n] = uint8(f.Offset)
		n++
	}
	if !hasFrac {
		copy(offs[:], offs[1:])
		n--
	}
	return offs, n, hasFrac
}

// maxRespellOffsets is how many positions an entry can carry: the fraction's
// dot, if it has one, and every literal byte after it.
const maxRespellOffsets = 9

// spelledCanonically reports whether s writes this entry's layout bytes the way
// the layout writes them, which is the question detection actually has, and the
// answer for nearly every input.
//
// It exists as a separate predicate because goLayoutFor cannot be inlinable and
// this can. Go charges 57 of an 80 budget for a call to a function it cannot
// inline, so anything that reaches the respelling work carries that call cost
// whatever shape it is given: goLayoutFor measured 89 calling an inlinable
// respellLiterals, 101 calling a noinline helper, and 95 with the fraction test
// folded into its loop. Three ways of moving the work, all over budget. What
// gets under it is not moving the work but not doing it: this asks yes or no,
// has no call in it at all, and the caller reaches goLayoutFor only for the
// input that answered no.
//
// The empty and length cases answer yes, which matches what goLayoutFor returns
// for them: the entry's own layout, which detectFormat compares equal.
//
// See P25. The 6.6 ns this is worth was measured when the function was first
// split, and `make codegen` is what noticed it had drifted back.
func spelledCanonically(e *formatEntry, s string) bool {
	if e.goLayout == "" || len(s) != len(e.goLayout) {
		return true
	}
	for _, off := range e.litOffsets[:e.nLits] {
		if s[off] != e.goLayout[off] {
			return false
		}
	}
	return true
}

// goLayoutFor returns the Go layout describing s, given the entry that matched
// it: the entry's own layout when s spells its literals the canonical way, and
// a copy with s's spelling substituted when it does not.
//
// The copy is the only allocation on this path and it happens for "2024/03/15"
// and not for "2024-03-15". A caller who takes GoLayout and hands it to
// time.Parse for the next row of their column gets a layout that reads it.
//
// Callers on the hot path ask spelledCanonically first and reach this only when
// the answer is no, so this one is not required to be inlinable and is not.
func goLayoutFor(e *formatEntry, s string) string {
	// The length test is what makes every index below safe, and the empty test
	// is for an entry that never had a layout: its offsets were never computed
	// and its fields are zero rather than absent.
	if e.goLayout == "" || len(s) != len(e.goLayout) {
		return e.goLayout
	}
	for i, off := range e.litOffsets[:e.nLits] {
		if s[off] != e.goLayout[off] {
			// Index 0 is the fraction's own dot when the entry has one, and it
			// is first for exactly this: an input that respells it gets no
			// layout, and gets it even when a separator is respelled too.
			if i == 0 && e.hasFrac {
				return ""
			}
			return respellLiterals(e, s)
		}
	}
	return e.goLayout
}

// respellLiterals copies the entry's layout with the input's literal bytes in
// it. The one allocation on this path, and it happens for "2024/03/15" and not
// for "2024-03-15".
//
// It skips the fraction's dot, which is why the slice is taken from 1: that
// position is not a separator to respell, and writing over it is what would
// turn ".000" into two literals. goLayoutFor has already refused the input
// where that byte differs, so what is skipped here is a byte that matched.
func respellLiterals(e *formatEntry, s string) string {
	b := []byte(e.goLayout)
	offs := e.litOffsets[:e.nLits]
	if e.hasFrac {
		offs = offs[1:]
	}
	for _, off := range offs {
		b[off] = s[off]
	}
	return string(b)
}

// stampLiteralClasses gives every one-byte literal in a trie entry the
// character class of the signature position it sits at.
//
// The entries in formats.go declare their literals with no Aux, which used to
// mean "any byte that is not a digit". That is the same check at a colon as at
// a dash, and NUMERIC_MDY and TIME_HMS are both DD?DD?DD, so each read the
// other's input and answered with a date where the input held a time or the
// reverse. The class each position matched on is already written down in the
// entry's signature; this copies it onto the field so the executor can ask for
// it too.
//
// Doing it here rather than in the 105 field literals keeps the class and the
// signature from drifting apart: an entry whose signature changes gets the new
// class without anybody remembering to.
//
// A literal wider than one byte is left alone. None exist in the trie, and one
// would need a class per byte rather than the one Aux holds.
func stampLiteralClasses(e *formatEntry) {
	for i := range e.fields {
		f := &e.fields[i]
		if f.Kind != compile.FLiteral || f.Len != 1 || f.Aux != 0 {
			continue
		}
		if int(f.Offset) >= len(e.sig) {
			continue
		}
		cc := e.sig[f.Offset]
		if cc == CDigit {
			continue
		}
		f.Aux = compile.AuxFor(litClassOf[cc])
	}
}
