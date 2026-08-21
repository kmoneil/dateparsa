package detect

import (
	"strings"
	"testing"
)

// scanReference is the classification Scan did before the table replaced the
// switch, kept verbatim so the table has something independent to disagree
// with. It is the switch from signature.go at 2d48388, writing into a slice
// instead of a packed key.
//
// This is the same discipline as lookupLinear in sigtable_test.go: a reference
// that shares machinery with the thing it checks cannot catch a mistake in that
// machinery, so this one shares none.
func scanReference(s string) (classes []CharClass, hasLetter bool) {
	n := len(s)
	if n > maxSigLen {
		n = maxSigLen
	}
	classes = make([]CharClass, n)
	for i := 0; i < n; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			classes[i] = CDigit
		case c == 'T':
			if i > 0 && i < n-1 && isDigit(s[i-1]) && isDigit(s[i+1]) {
				classes[i] = CSpecial
			} else {
				classes[i] = CLetter
				hasLetter = true
			}
		case c == 'Z':
			if i == n-1 || (i < n-1 && (s[i+1] == '+' || s[i+1] == '-')) {
				classes[i] = CSpecial
			} else {
				classes[i] = CLetter
				hasLetter = true
			}
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Y'):
			classes[i] = CLetter
			hasLetter = true
		case c == '-' || c == '/':
			if c == '-' && i > 0 && isTZSignPosition(s, i, n) {
				classes[i] = CSpecial
			} else {
				classes[i] = CSep
			}
		case c == '.':
			classes[i] = CSep
		case c == ' ' || c == '\t':
			classes[i] = CSpace
		case c == ':':
			classes[i] = CColon
		case c == '+':
			classes[i] = CSpecial
		case c == ',':
			classes[i] = CSpecial
		default:
			classes[i] = CLetter
			hasLetter = true
		}
	}
	return classes, hasLetter
}

// TestScanMatchesReferenceOnEveryByte walks all 256 byte values at the
// positions where context can change the answer: first, last, between digits,
// before a sign, and after a time. A table built wrong for one byte is a format
// detected wrong for every input containing it.
func TestScanMatchesReferenceOnEveryByte(t *testing.T) {
	contexts := []struct{ pre, post string }{
		{"", ""},
		{"1", "2"},
		{"2024-03-15T10:30:00", ""},
		{"2024-03-15T10:30:00", "+05:30"},
		{"2024-03-15T10:30:00", "-05:00"},
		{"abc", "def"},
		{"12:34:56", "07:00"},
		{" ", " "},
	}
	for _, ctx := range contexts {
		for b := 0; b < 256; b++ {
			in := ctx.pre + string([]byte{byte(b)}) + ctx.post
			sig := Scan(in)
			want, wantLetter := scanReference(in)
			if sig.Len() != len(want) {
				t.Fatalf("%q: Len %d, want %d", in, sig.Len(), len(want))
			}
			for i := range want {
				if got := sig.At(i); got != want[i] {
					t.Fatalf("%q: position %d is %d, reference says %d", in, i, got, want[i])
				}
			}
			if sig.HasLetter != wantLetter {
				t.Fatalf("%q: HasLetter %v, reference says %v", in, sig.HasLetter, wantLetter)
			}
		}
	}
}

// FuzzScanMatchesReference is the same property over whatever the corpus
// reaches, including inputs past maxSigLen and non-ASCII bytes.
func FuzzScanMatchesReference(f *testing.F) {
	for _, s := range []string{
		"2024-03-15", "2024-03-15T10:30:00Z", "2024-03-15T10:30:00-08:00",
		"10:30:00.123456789+05:30", "March 15, 2024", "März 15", "2014年04月08日",
		"", "T", "Z", "-", "TZ-", strings.Repeat("Z", 70),
		strings.Repeat("2024-03-15T10:30:00Z", 5),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		sig := Scan(s)
		want, wantLetter := scanReference(s)
		if sig.Len() != len(want) {
			t.Fatalf("%q: Len %d, want %d", s, sig.Len(), len(want))
		}
		for i := range want {
			if got := sig.At(i); got != want[i] {
				t.Fatalf("%q: position %d is %d, reference says %d", s, i, got, want[i])
			}
		}
		if sig.HasLetter != wantLetter {
			t.Fatalf("%q: HasLetter %v, reference says %v", s, sig.HasLetter, wantLetter)
		}
	})
}
