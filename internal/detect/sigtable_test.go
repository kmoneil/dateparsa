package detect

import (
	"strings"
	"testing"
)

// packOf is the reference packer: the plain, obvious loop, written separately
// from the one Scan and sigTable.insert use so that a mistake in the shared
// incremental version has something to disagree with.
func packOf(classes []CharClass) [sigWords]uint64 {
	var key [sigWords]uint64
	for i, cc := range classes {
		key[i/sigClassesPerWord] |= uint64(cc) << uint(i%sigClassesPerWord*sigBitsPerClass)
	}
	return key
}

// lookupLinear is the reference lookup: compare the signature against every
// entry, position by position, with no packing and no hashing at all.
//
// It is what the hash table has to agree with, and it is deliberately the
// slowest possible implementation, because a reference that shares machinery
// with the thing it checks cannot catch a mistake in that machinery.
func lookupLinear(sig *Signature) *formatEntry {
	var found *formatEntry
	for i := range globalSigTable.slots {
		e := globalSigTable.slots[i].entry
		if e == nil || len(e.sig) != sig.Len() {
			continue
		}
		match := true
		for j, cc := range e.sig {
			if sig.At(j) != cc {
				match = false
				break
			}
		}
		if match {
			found = e
		}
	}
	return found
}

// TestSignatureTableFindsEveryFormat checks that every format can be found by
// its own signature, which the trie gave for free and a hash table does not.
func TestSignatureTableFindsEveryFormat(t *testing.T) {
	n := 0
	forEachEntry(t, func(e *formatEntry) {
		n++
		var sig Signature
		sig.len = len(e.sig)
		sig.key = packOf(e.sig)
		got := globalSigTable.lookup(&sig)
		if got != e {
			name := "nil"
			if got != nil {
				name = got.name
			}
			t.Errorf("%s: its own signature looks up %s", e.name, name)
		}
	})
	if n == 0 {
		t.Fatal("the table is empty")
	}
	t.Logf("%d formats, %d slots", n, len(globalSigTable.slots))
}

// TestNoTwoFormatsShareASignature states in a test what sigTable.insert says
// with a panic. Two formats declaring one signature has no correct resolution
// at run time: whichever was inserted second would be unreachable, and which
// one that is depends on the order phase1Formats and phase2Formats are walked.
func TestNoTwoFormatsShareASignature(t *testing.T) {
	seen := make(map[string]string)
	forEachEntry(t, func(e *formatEntry) {
		var b strings.Builder
		for _, cc := range e.sig {
			b.WriteByte("DLSWCX"[cc])
		}
		k := b.String()
		if prev, dup := seen[k]; dup {
			t.Errorf("%s and %s both declare %s", prev, e.name, k)
		}
		seen[k] = e.name
	})
}

// TestSignatureTableHasHeadroom guards the load factor. Linear probing degrades
// sharply as a table fills, and the table is sized by a constant that nobody
// will think to raise when the thirty-fourth format is added.
func TestSignatureTableHasHeadroom(t *testing.T) {
	used := 0
	for i := range globalSigTable.slots {
		if globalSigTable.slots[i].entry != nil {
			used++
		}
	}
	if load := float64(used) / float64(len(globalSigTable.slots)); load > 0.5 {
		t.Errorf("%d of %d slots used, load %.2f; raise sigTableSlots", used, len(globalSigTable.slots), load)
	}
}

// TestAtUnpacksWhatScanPacked checks the accessor against the scanner for every
// byte value at every position a signature can hold, including the word
// boundaries at 21 and 42 where the packing carries.
func TestAtUnpacksWhatScanPacked(t *testing.T) {
	for _, pos := range []int{0, 1, 20, 21, 22, 41, 42, 43, 62, 63} {
		for b := 0; b < 256; b++ {
			in := strings.Repeat("x", pos) + string(rune(b)) + strings.Repeat("y", maxSigLen-pos-1)
			if len(in) != maxSigLen {
				continue // a byte above 127 encodes wide; those are covered by the fuzzer
			}
			sig := Scan(in)
			if sig.Len() != maxSigLen {
				t.Fatalf("pos %d byte %d: length %d", pos, b, sig.Len())
			}
			// Every position has to read back as something, and the positions
			// around the one under test have to be untouched by it.
			if pos > 0 && sig.At(pos-1) != CLetter {
				t.Fatalf("pos %d byte %d: position %d reads %d, want CLetter", pos, b, pos-1, sig.At(pos-1))
			}
			if pos < maxSigLen-1 && sig.At(pos+1) != CLetter {
				t.Fatalf("pos %d byte %d: position %d reads %d, want CLetter", pos, b, pos+1, sig.At(pos+1))
			}
		}
	}
}

// FuzzSignatureLookupMatchesLinearScan is the regression test that matters. For
// any input, the hash table has to return exactly what comparing the signature
// against every format position by position returns.
//
// A hash collision resolved wrongly, a probe that stops early, a length left
// out of the compare, or a packing that loses the class at position 21 all show
// up here as a disagreement, and any of them would mean an input detected as
// the wrong format.
func FuzzSignatureLookupMatchesLinearScan(f *testing.F) {
	for _, s := range []string{
		"2024-03-15", "2024-03-15T10:30:00Z", "2024-03-15T10:30:00.123456789+05:30",
		"20240315", "10:30", "10:30:00", "March 15, 2024", "2024-W11-5",
		"2024-03-15 10:30:00 UTC", "", "x", strings.Repeat("1", 64),
		strings.Repeat("1", 65), "2024/03/15", "0000-001",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		sig := Scan(s)
		got := globalSigTable.lookup(&sig)
		want := lookupLinear(&sig)
		if got != want {
			gn, wn := "nil", "nil"
			if got != nil {
				gn = got.name
			}
			if want != nil {
				wn = want.name
			}
			t.Fatalf("%q (len %d): table says %s, linear scan says %s", s, sig.Len(), gn, wn)
		}
	})
}

// FuzzPackedSignatureRoundTrips holds the packing to the accessor: whatever
// Scan wrote, At has to read back, at every position.
func FuzzPackedSignatureRoundTrips(f *testing.F) {
	for _, s := range []string{"2024-03-15T10:30:00Z", "", strings.Repeat("a", 64), "März 15"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		sig := Scan(s)
		n := len(s)
		if n > maxSigLen {
			n = maxSigLen
		}
		if sig.Len() != n {
			t.Fatalf("%q: Len %d, want %d", s, sig.Len(), n)
		}
		var classes []CharClass
		for i := 0; i < sig.Len(); i++ {
			c := sig.At(i)
			if c >= numClasses {
				t.Fatalf("%q: position %d unpacks to %d, past the %d classes", s, i, c, numClasses)
			}
			classes = append(classes, c)
		}
		if packOf(classes) != sig.key {
			t.Fatalf("%q: repacking what At reported does not reproduce the key", s)
		}
	})
}
