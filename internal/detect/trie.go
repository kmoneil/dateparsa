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

// build constructs the trie from all known format definitions.
func buildTrie() *trie {
	t := &trie{}
	for _, formats := range [][]formatEntry{phase1Formats(), phase2Formats()} {
		for i := range formats {
			if len(formats[i].sig) > 0 {
				stampLiteralClasses(&formats[i])
				formats[i].litOffsets, formats[i].nLits, formats[i].fracOffset = literalOffsets(formats[i].goLayout)
				// Pre-build the FormatDef so Detect doesn't allocate one per call.
				if !formats[i].ambig && len(formats[i].fields) > 0 {
					formats[i].def = &compile.FormatDef{
						Name:     formats[i].name,
						GoLayout: formats[i].goLayout,
						Fields:   formats[i].fields,
					}
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
// A layout it refuses gets no offsets and no check, which leaves that entry
// exactly as it was.
// A layout with more literals than the array holds gets none and no check,
// which leaves that entry exactly as it was. Eight covers every entry in the
// tree: the widest is RFC3339_NANO with seven.
func literalOffsets(goLayout string) (offs [8]uint8, n uint8, frac uint8) {
	frac = noFracOffset
	if goLayout == "" || len(goLayout) > 255 {
		return offs, 0, frac
	}
	def, err := compile.ParseGoLayout(goLayout)
	if err != nil {
		return offs, 0, frac
	}
	// The "." of a fraction is a literal to the layout parser and is not one to
	// respell. ".000" is a token: write the input's "/" over its dot and Go
	// reads a literal "/" followed by a literal "000", which parses
	// "00:00:00/000" and refuses "00:00:00/010". A layout that takes one value
	// of the field it describes is worse than no layout, so the entry reports
	// none for an input spelled that way.
	fracAt := -1
	for _, f := range def.Fields {
		if f.Kind == compile.FFracSec && f.Offset > 0 {
			fracAt = int(f.Offset) - 1
		}
	}
	for _, f := range def.Fields {
		if f.Kind != compile.FLiteral {
			continue
		}
		if int(f.Offset) == fracAt {
			frac = uint8(f.Offset)
			continue
		}
		if int(n) == len(offs) {
			return [8]uint8{}, 0, noFracOffset
		}
		offs[n] = uint8(f.Offset)
		n++
	}
	return offs, n, frac
}

// noFracOffset is fracOffset's "this layout has no fraction" value. 255 rather
// than -1 so the field stays a byte, and a layout is at most 255 bytes long, so
// no real offset can collide with it.
const noFracOffset = 255

// goLayoutFor returns the Go layout describing s, given the entry that matched
// it: the entry's own layout when s spells its literals the canonical way, and
// a copy with s's spelling substituted when it does not.
//
// The copy is the only allocation on this path and it happens for
// "2024/03/15" and not for "2024-03-15". A caller who takes GoLayout and hands
// it to time.Parse for the next row of their column gets a layout that reads
// it.
// The rewrite is a second function so that this one stays inlinable. It was one
// function at cost 81 against a budget of 80, the call cost 6.6ns on
// Detect_Only, and splitting the branch nobody takes out of the branch everyone
// takes is what C17's fraction check did for the same reason.
func goLayoutFor(e *formatEntry, s string) string {
	// The length test is what makes every index below safe, and the empty test
	// is for an entry that never had a layout: its offsets were never computed
	// and its fields are zero rather than absent.
	if e.goLayout == "" || len(s) != len(e.goLayout) {
		return e.goLayout
	}
	if e.fracOffset != noFracOffset && s[e.fracOffset] != e.goLayout[e.fracOffset] {
		return ""
	}
	for _, off := range e.litOffsets[:e.nLits] {
		if s[off] != e.goLayout[off] {
			return respellLiterals(e, s)
		}
	}
	return e.goLayout
}

// respellLiterals copies the entry's layout with the input's literal bytes in
// it. The one allocation on this path, and it happens for "2024/03/15" and not
// for "2024-03-15".
func respellLiterals(e *formatEntry, s string) string {
	b := []byte(e.goLayout)
	for _, off := range e.litOffsets[:e.nLits] {
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
