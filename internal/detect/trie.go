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
