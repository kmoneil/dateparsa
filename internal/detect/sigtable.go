package detect

import "github.com/kmoneil/dateparsa/internal/compile"

// sigTable maps a packed signature to the format that declares it.
//
// It replaced a trie, and the reason is what a trie costs on this shape of key.
// A trie walks one node per input position, and each node is a pointer the
// previous node held, so a twenty-byte timestamp was twenty dependent loads
// through 139 nodes of 56 bytes: nothing to overlap them with and a cache miss
// available at every step. The whole key fits in four words, so there is
// nothing to walk.
//
// Open addressing with linear probing, sized to a power of two well above the
// number of entries, so a hit is one probe and a miss is one or two. Built once
// at init and never written afterwards, which is what makes it safe to read
// from every goroutine with no exclusion.
type sigTable struct {
	slots []sigSlot
	mask  uint64
}

type sigSlot struct {
	key   [sigWords]uint64
	n     int32
	entry *formatEntry
}

// sigHash mixes the packed key and the length into a bucket index.
//
// The length is part of the key and not just of the compare. Two signatures can
// pack identically and differ in length, because a class is three bits and zero
// is CDigit: "DD" and "DDDD" are 0 in every word that matters, so length is the
// only thing separating them. The compare below checks it too; this only has to
// spread them.
func sigHash(key *[sigWords]uint64, n int) uint64 {
	h := uint64(n) * 0x9E3779B97F4A7C15
	for _, w := range key {
		h ^= w
		h *= 0xC2B2AE3D27D4EB4F
	}
	return h ^ h>>29
}

// lookup returns the entry whose signature is exactly this one, or nil.
func (t *sigTable) lookup(sig *Signature) *formatEntry {
	i := sigHash(&sig.key, sig.len) & t.mask
	for {
		sl := &t.slots[i]
		if sl.entry == nil {
			return nil
		}
		if sl.n == int32(sig.len) && sl.key == sig.key {
			return sl.entry
		}
		i = (i + 1) & t.mask
	}
}

// insert adds an entry under its own signature. It panics on a collision
// because two formats declaring the same signature is a defect in formats.go
// that has no correct resolution at run time, and this runs at init, so the
// panic is a build-time failure in every practical sense.
// TestNoTwoFormatsShareASignature says the same thing where a reader will see it.
func (t *sigTable) insert(e *formatEntry) {
	var key [sigWords]uint64
	w, sh := 0, uint(0)
	for _, cc := range e.sig {
		key[w] |= uint64(cc) << sh
		sh += sigBitsPerClass
		if sh >= sigClassesPerWord*sigBitsPerClass {
			w, sh = w+1, 0
		}
	}
	n := int32(len(e.sig))

	i := sigHash(&key, int(n)) & t.mask
	for t.slots[i].entry != nil {
		if t.slots[i].n == n && t.slots[i].key == key {
			panic("detect: two formats declare the signature of " + e.name)
		}
		i = (i + 1) & t.mask
	}
	t.slots[i] = sigSlot{key: key, n: n, entry: e}
}

// sigTableSlots is the table's length, a power of two comfortably above the
// number of formats so that probing stays short. 33 entries in 128 slots is a
// load factor of a quarter.
const sigTableSlots = 128

// prebuiltDefs holds every FormatDef buildTrie prebuilt, which are the defs a
// Result carries by pointer rather than building per call.
var prebuiltDefs []*compile.FormatDef

// PrebuiltDefs returns the FormatDefs a signature-table hit hands back by
// pointer.
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

// buildSigTable constructs the signature table from all known format
// definitions.
func buildSigTable() *sigTable {
	t := &sigTable{slots: make([]sigSlot, sigTableSlots), mask: sigTableSlots - 1}
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

// stampLiteralClasses gives every one-byte literal in a format entry the
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
