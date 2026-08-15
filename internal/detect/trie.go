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
		if f.Offset >= len(e.sig) {
			continue
		}
		cc := e.sig[f.Offset]
		if cc == CDigit {
			continue
		}
		f.Aux = compile.AuxFor(litClassOf[cc])
	}
}
